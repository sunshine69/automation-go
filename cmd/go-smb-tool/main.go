package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hirochachacha/go-smb2"
	u "github.com/sunshine69/golang-tools/utils"
)

// Global flags
var (
	// Active flags
	serverFlag  string
	loginUser   string
	loginPass   string
	smbDomain   string
	verboseFlag bool

	// Deprecated/ignored flags kept for backward compatibility
	_ string // tokenFlag  -tok
	_ string // configFlag -config
)

func main() {
	// Main flagset
	mainFlagSet := flag.NewFlagSet("smbutil", flag.ExitOnError)

	mainFlagSet.StringVar(&serverFlag, "server", "", "SMB server address (hostname:port). Default SMB port is 445.")
	mainFlagSet.StringVar(&loginUser, "login", "", "Login user for the SMB share")
	mainFlagSet.StringVar(&loginPass, "password", "", "Login password (can also be set via SMB_PASSWORD env var)")
	mainFlagSet.StringVar(&smbDomain, "domain", "", "Domain for SMB authentication")
	mainFlagSet.BoolVar(&verboseFlag, "verbose", false, "Enable verbose output")

	// Backward-compatible no-op flags (silently ignored)
	var ignoredTok, ignoredConfig string
	mainFlagSet.StringVar(&ignoredTok, "tok", "", "[DEPRECATED] JWT token - no longer used, ignored")
	mainFlagSet.StringVar(&ignoredConfig, "config", "", "[DEPRECATED] Config file path - no longer used, ignored")

	// Subcommands
	uploadCmd := flag.NewFlagSet("upload", flag.ExitOnError)
	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
	mvCmd := flag.NewFlagSet("mv", flag.ExitOnError)
	rmCmd := flag.NewFlagSet("rm", flag.ExitOnError)
	lsCmd := flag.NewFlagSet("ls", flag.ExitOnError)
	cleanCmd := flag.NewFlagSet("clean", flag.ExitOnError)

	var (
		uploadSource string
		uploadDest   string
	)
	uploadCmd.StringVar(&uploadSource, "src", "-", "Source file or directory (use - for stdin, single file only)")
	uploadCmd.StringVar(&uploadDest, "dest", "", "Destination path on SMB share (e.g. /sharename/path/to/file.txt or /sharename/path/to/dir)")

	var (
		downloadSource string
		downloadDest   string
	)
	downloadCmd.StringVar(&downloadSource, "src", "", "Source file or directory on SMB share")
	downloadCmd.StringVar(&downloadDest, "dest", "-", "Destination local file or directory (use - for stdout, single file only)")

	var (
		mvSource string
		mvDest   string
	)
	mvCmd.StringVar(&mvSource, "src", "", "Source file on SMB share")
	mvCmd.StringVar(&mvDest, "dest", "", "Destination path on SMB share")

	var rmPath, lsPath string
	var rmForce bool
	rmCmd.StringVar(&rmPath, "path", "", "File or directory to remove on SMB share")
	rmCmd.BoolVar(&rmForce, "force", false, "Required to remove a directory; deletes it and all contents recursively")
	lsCmd.StringVar(&lsPath, "path", "", "Path/glob pattern to list on SMB share")

	var (
		cleanPath u.ArrayFlags
		days      int
		dryRun    bool
	)
	cleanCmd.Var(&cleanPath, "path", "List of Path/glob pattern to clean on SMB share.")
	cleanCmd.IntVar(&days, "days", 90, "Delete files older than X days")
	cleanCmd.BoolVar(&dryRun, "dry-run", false, "List files without deleting them")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Split args into main flags and subcommand args
	mainArgs := []string{}
	subCmdPos := len(os.Args) // default: no subcommand found
	for i, arg := range os.Args[1:] {
		if arg == "upload" || arg == "download" || arg == "mv" || arg == "rm" || arg == "ls" || arg == "clean" {
			subCmdPos = i + 1
			break
		}
		mainArgs = append(mainArgs, arg)
	}

	if err := mainFlagSet.Parse(mainArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	// Password fallback to env var
	if loginPass == "" {
		loginPass = os.Getenv("SMB_PASSWORD")
	}

	// Validate required flags
	if serverFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: -server is required")
		os.Exit(1)
	}
	if loginUser == "" {
		fmt.Fprintln(os.Stderr, "Error: -login is required")
		os.Exit(1)
	}
	if smbDomain == "" {
		fmt.Fprintln(os.Stderr, "Error: -domain is required")
		os.Exit(1)
	}

	if subCmdPos >= len(os.Args) {
		fmt.Fprintln(os.Stderr, "Error: expected subcommand: upload, download, mv, rm, ls, clean")
		printUsage()
		os.Exit(1)
	}

	var err error

	switch os.Args[subCmdPos] {
	case "upload":
		uploadCmd.Parse(os.Args[subCmdPos+1:])
		if uploadDest == "" {
			fmt.Fprintln(os.Stderr, "Error: -dest is required for upload")
			uploadCmd.PrintDefaults()
			os.Exit(1)
		}
		err = upload(serverFlag, uploadSource, uploadDest, verboseFlag)

	case "download":
		downloadCmd.Parse(os.Args[subCmdPos+1:])
		if downloadSource == "" {
			fmt.Fprintln(os.Stderr, "Error: -src is required for download")
			downloadCmd.PrintDefaults()
			os.Exit(1)
		}
		err = download(serverFlag, downloadSource, downloadDest, verboseFlag)

	case "mv":
		mvCmd.Parse(os.Args[subCmdPos+1:])
		if mvSource == "" || mvDest == "" {
			fmt.Fprintln(os.Stderr, "Error: -src and -dest are required for mv")
			mvCmd.PrintDefaults()
			os.Exit(1)
		}
		err = moveFile(serverFlag, mvSource, mvDest, verboseFlag)

	case "rm":
		rmCmd.Parse(os.Args[subCmdPos+1:])
		if rmPath == "" {
			fmt.Fprintln(os.Stderr, "Error: -path is required for rm")
			rmCmd.PrintDefaults()
			os.Exit(1)
		}
		err = removeFile(serverFlag, rmPath, rmForce, verboseFlag)

	case "ls":
		lsCmd.Parse(os.Args[subCmdPos+1:])
		if lsPath == "" {
			fmt.Fprintln(os.Stderr, "Error: -path is required for ls")
			lsCmd.PrintDefaults()
			os.Exit(1)
		}
		files, lsErr := listFiles(serverFlag, lsPath)
		if lsErr != nil {
			fmt.Fprintf(os.Stderr, "Operation failed: %v\n", lsErr)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, u.JsonDump(files, ""))

	case "clean":
		cleanCmd.Parse(os.Args[subCmdPos+1:])
		if len(cleanPath) == 0 || days < 0 {
			fmt.Fprintln(os.Stderr, "Error: -path and -days >= 0 are required for clean")
			cleanCmd.PrintDefaults()
			os.Exit(1)
		}
		for _, mypath := range cleanPath {
			fmt.Fprintf(os.Stderr, "Run clean for '%s'\n", mypath)
			err = cleanOldFiles(serverFlag, mypath, days, dryRun, verboseFlag)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", os.Args[subCmdPos])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Operation failed: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: go-smb-tool [flags] <subcommand> [subcommand flags]

Flags:
  -server  <host:port>   SMB server address (port 445 is default for SMB)
  -login   <user>        SMB login username
  -password <pass>       SMB login password (or set SMB_PASSWORD env var)
  -domain  <domain>      SMB domain
  -verbose               Enable verbose output

  -tok     <token>       [DEPRECATED] Ignored, kept for backward compatibility
  -config  <path>        [DEPRECATED] Ignored, kept for backward compatibility

Subcommands:
  upload   -src <local_file|local_dir|-stdin> -dest </share/path>
           If -src is a local directory, its entire contents are uploaded
           recursively, preserving the directory structure under -dest.
           stdin (-src -) only supports a single file.

  download -src </share/path> -dest <local_file|local_dir|-stdout>
           If -src is a remote directory, its entire contents are downloaded
           recursively, preserving the directory structure under -dest.
           stdout (-dest -) only supports a single file.

  mv       -src </share/path> -dest </share/path>
  rm       -path </share/path> [-force]
           Removes a single file. If -path is a directory, -force is
           required and the directory is deleted recursively along with
           all of its contents.
  ls       -path </share/glob_pattern>
  clean    -path <path> -days <X> [-dry-run] Clean files older than X days

Examples:
  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    upload -src ./file.txt -dest /sharename/tmp/file.txt

  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    upload -src ./localdir -dest /sharename/tmp/remotedir

  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    download -src /sharename/tmp/file.txt -dest -

  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    download -src /sharename/tmp/remotedir -dest ./localdir

  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    rm -path /sharename/tmp/olddir -force

  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    ls -path /sharename/tmp/*.txt

  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    clean -path /sharename/tmp/*.log -days 7 -dry-run`)
}

// connectToSMB establishes a connection to the SMB server
func connectToSMB(server, username, password, domain string) (*smb2.Session, error) {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %v", err)
	}

	dialer := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     username,
			Password: password,
			Domain:   domain,
		},
	}

	session, err := dialer.Dial(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SMB authentication failed: %v", err)
	}

	return session, nil
}

// parseSharePath splits "/sharename/path/to/file" into ("sharename", "path/to/file")
func parseSharePath(path string) (string, string, error) {
	if !strings.HasPrefix(path, "/") {
		return "", "", fmt.Errorf("path must start with /sharename/...")
	}

	path = path[1:] // strip leading slash
	parts := strings.SplitN(path, "/", 2)
	shareName := parts[0]

	filePath := ""
	if len(parts) > 1 {
		filePath = parts[1]
	}

	return shareName, filePath, nil
}

// upload uploads a local file/stdin or an entire local directory tree to an SMB share
func upload(server, srcPath, destPath string, verbose bool) error {
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return err
	}
	defer session.Logoff()

	shareName, filePath, err := parseSharePath(destPath)
	if err != nil {
		return err
	}

	share, err := session.Mount(shareName)
	if err != nil {
		return fmt.Errorf("failed to mount share %s: %v", shareName, err)
	}
	defer share.Umount()

	if srcPath != "-" {
		if info, statErr := os.Stat(srcPath); statErr == nil && info.IsDir() {
			return uploadDirectory(share, srcPath, filePath, verbose)
		} else if statErr != nil {
			return fmt.Errorf("failed to stat source path %s: %v", srcPath, statErr)
		}
	}

	return uploadSingleFile(share, srcPath, filePath, verbose)
}

// uploadSingleFile uploads one local file (or stdin) to a remote path on an
// already-mounted share, creating any needed remote directories first.
func uploadSingleFile(share *smb2.Share, srcPath, remoteFilePath string, verbose bool) error {
	dirPath := filepath.Dir(remoteFilePath)
	if dirPath != "." {
		if err := createDirectories(share, dirPath); err != nil {
			return fmt.Errorf("failed to create directories: %v", err)
		}
	}

	destFile, err := share.Create(remoteFilePath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer destFile.Close()

	var srcFile io.Reader
	if srcPath == "-" {
		srcFile = bufio.NewReader(os.Stdin)
		if verbose {
			fmt.Fprintln(os.Stderr, "Reading from stdin...")
		}
	} else {
		f, err := os.Open(srcPath)
		if err != nil {
			return fmt.Errorf("failed to open source file: %v", err)
		}
		defer f.Close()
		srcFile = f
		if verbose {
			fmt.Fprintf(os.Stderr, "Reading from file: %s\n", srcPath)
		}
	}

	bytesWritten, err := io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy data: %v", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Uploaded %d bytes to %s\n", bytesWritten, remoteFilePath)
	}
	return nil
}

// uploadDirectory walks a local directory tree and uploads every regular
// file to the remote share, preserving the relative directory structure
// under remoteBase.
func uploadDirectory(share *smb2.Share, localDir, remoteBase string, verbose bool) error {
	localDir = filepath.Clean(localDir)

	return filepath.Walk(localDir, func(localPath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("failed to walk %s: %v", localPath, walkErr)
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(localDir, localPath)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %s: %v", localPath, err)
		}
		relPath = filepath.ToSlash(relPath)

		remoteFilePath := strings.TrimSuffix(remoteBase, "/") + "/" + relPath
		remoteFilePath = strings.TrimPrefix(remoteFilePath, "/")

		if verbose {
			fmt.Fprintf(os.Stderr, "Uploading %s -> %s\n", localPath, remoteFilePath)
		}

		if err := uploadSingleFile(share, localPath, remoteFilePath, verbose); err != nil {
			return fmt.Errorf("failed to upload %s: %v", localPath, err)
		}
		return nil
	})
}

// download downloads a remote file or an entire remote directory tree from an SMB share
func download(server, srcPath, destPath string, verbose bool) error {
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return err
	}
	defer session.Logoff()

	shareName, filePath, err := parseSharePath(srcPath)
	if err != nil {
		return err
	}

	share, err := session.Mount(shareName)
	if err != nil {
		return fmt.Errorf("failed to mount share %s: %v", shareName, err)
	}
	defer share.Umount()

	info, err := share.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat source path %s: %v", srcPath, err)
	}

	if info.IsDir() {
		if destPath == "-" {
			return fmt.Errorf("cannot download a directory to stdout; specify a local directory for -dest")
		}
		return downloadDirectory(share, filePath, destPath, verbose)
	}

	return downloadSingleFile(share, filePath, destPath, verbose)
}

// downloadSingleFile downloads one remote file to a local file or stdout,
// on an already-mounted share.
func downloadSingleFile(share *smb2.Share, remoteFilePath, destPath string, verbose bool) error {
	srcFile, err := share.Open(remoteFilePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer srcFile.Close()

	var destFile io.Writer
	if destPath == "-" {
		destFile = os.Stdout
		if verbose {
			fmt.Fprintln(os.Stderr, "Writing to stdout...")
		}
	} else {
		if dirPath := filepath.Dir(destPath); dirPath != "." {
			if err := os.MkdirAll(dirPath, 0755); err != nil {
				return fmt.Errorf("failed to create local directory %s: %v", dirPath, err)
			}
		}
		f, err := os.Create(destPath)
		if err != nil {
			return fmt.Errorf("failed to create destination file: %v", err)
		}
		defer f.Close()
		destFile = f
		if verbose {
			fmt.Fprintf(os.Stderr, "Writing to file: %s\n", destPath)
		}
	}

	bytesRead, err := io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy data: %v", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Downloaded %d bytes from %s\n", bytesRead, remoteFilePath)
	}
	return nil
}

// downloadDirectory recursively downloads every file under a remote
// directory to a local directory, preserving the relative structure.
func downloadDirectory(share *smb2.Share, remoteDir, localDir string, verbose bool) error {
	remoteDir = strings.TrimSuffix(filepath.ToSlash(remoteDir), "/")

	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory %s: %v", localDir, err)
	}

	return walkRemoteDir(share, remoteDir, "", func(relPath string, info os.FileInfo) error {
		localPath := filepath.Join(localDir, filepath.FromSlash(relPath))

		if info.IsDir() {
			if err := os.MkdirAll(localPath, 0755); err != nil {
				return fmt.Errorf("failed to create local directory %s: %v", localPath, err)
			}
			return nil
		}

		remotePath := remoteDir + "/" + relPath
		if verbose {
			fmt.Fprintf(os.Stderr, "Downloading %s -> %s\n", remotePath, localPath)
		}
		if err := downloadSingleFile(share, remotePath, localPath, verbose); err != nil {
			return fmt.Errorf("failed to download %s: %v", remotePath, err)
		}
		return nil
	})
}

// walkRemoteDir recursively enumerates a remote directory using Open+Readdir
// (the go-smb2 equivalent of os.Open + File.Readdir) and invokes fn for
// every entry (both directories and files) found, with relPath expressed
// relative to remoteBase using forward slashes.
func walkRemoteDir(share *smb2.Share, remoteBase, relPath string, fn func(relPath string, info os.FileInfo) error) error {
	remotePath := remoteBase
	if relPath != "" {
		remotePath = remoteBase + "/" + relPath
	}

	dir, err := share.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote directory %s: %v", remotePath, err)
	}
	entries, err := dir.Readdir(-1)
	dir.Close()
	if err != nil {
		return fmt.Errorf("failed to read remote directory %s: %v", remotePath, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." {
			continue
		}

		childRel := name
		if relPath != "" {
			childRel = relPath + "/" + name
		}

		if entry.IsDir() {
			if err := fn(childRel, entry); err != nil {
				return err
			}
			if err := walkRemoteDir(share, remoteBase, childRel, fn); err != nil {
				return err
			}
		} else {
			if err := fn(childRel, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

// moveFile moves/renames a file within or across SMB shares
func moveFile(server, srcPath, destPath string, verbose bool) error {
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return err
	}
	defer session.Logoff()

	srcShareName, srcFilePath, err := parseSharePath(srcPath)
	if err != nil {
		return err
	}
	destShareName, destFilePath, err := parseSharePath(destPath)
	if err != nil {
		return err
	}

	if srcShareName == destShareName {
		share, err := session.Mount(srcShareName)
		if err != nil {
			return fmt.Errorf("failed to mount share %s: %v", srcShareName, err)
		}
		defer share.Umount()

		dirPath := filepath.Dir(destFilePath)
		if dirPath != "." {
			if err = createDirectories(share, dirPath); err != nil {
				return fmt.Errorf("failed to create directories: %v", err)
			}
		}

		if err = share.Rename(srcFilePath, destFilePath); err != nil {
			return fmt.Errorf("failed to rename file: %v", err)
		}
	} else {
		// Cross-share move: copy via temp file then delete source
		tempFile, err := os.CreateTemp("", "smb-move-*")
		if err != nil {
			return fmt.Errorf("failed to create temporary file: %v", err)
		}
		tempFileName := tempFile.Name()
		tempFile.Close()
		defer os.Remove(tempFileName)

		if err = download(server, srcPath, tempFileName, false); err != nil {
			return fmt.Errorf("failed to download source file: %v", err)
		}
		if err = upload(server, tempFileName, destPath, false); err != nil {
			return fmt.Errorf("failed to upload to destination: %v", err)
		}
		if err = removeFile(server, srcPath, false, false); err != nil {
			return fmt.Errorf("warning: failed to remove source file: %v", err)
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Moved %s to %s\n", srcPath, destPath)
	}
	return nil
}

// removeFile deletes a file or, with force=true, a directory (and all its
// contents, recursively) from an SMB share.
func removeFile(server, path string, force, verbose bool) error {
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return err
	}
	defer session.Logoff()

	shareName, filePath, err := parseSharePath(path)
	if err != nil {
		return fmt.Errorf("failed to parse path: %v", err)
	}

	share, err := session.Mount(shareName)
	if err != nil {
		return fmt.Errorf("failed to mount share %s: %v", shareName, err)
	}
	defer share.Umount()

	info, err := share.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %v", path, err)
	}

	if info.IsDir() {
		if !force {
			return fmt.Errorf("%s is a directory; pass -force to remove it and its contents recursively", path)
		}
		if err := removeRemoteRecursive(share, filePath, verbose); err != nil {
			return err
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Removed directory (recursive): %s\n", path)
		}
		return nil
	}

	if err = share.Remove(filePath); err != nil {
		return fmt.Errorf("failed to remove file %s: %v", filePath, err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Removed %s\n", path)
	}
	return nil
}

// removeRemoteRecursive deletes every entry under a remote directory
// (depth-first, files before directories) and then removes the directory
// itself. Uses Open+Readdir per entry, same primitive as walkRemoteDir,
// but processes bottom-up since a directory must be empty before it can
// be removed.
func removeRemoteRecursive(share *smb2.Share, remotePath string, verbose bool) error {
	dir, err := share.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote directory %s: %v", remotePath, err)
	}
	entries, err := dir.Readdir(-1)
	dir.Close()
	if err != nil {
		return fmt.Errorf("failed to read remote directory %s: %v", remotePath, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." {
			continue
		}
		childPath := remotePath + "/" + name

		if entry.IsDir() {
			if err := removeRemoteRecursive(share, childPath, verbose); err != nil {
				return err
			}
		} else {
			if err := share.Remove(childPath); err != nil {
				return fmt.Errorf("failed to remove file %s: %v", childPath, err)
			}
			if verbose {
				fmt.Fprintf(os.Stderr, "Removed: %s\n", childPath)
			}
		}
	}

	if err := share.Remove(remotePath); err != nil {
		return fmt.Errorf("failed to remove directory %s: %v", remotePath, err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Removed directory: %s\n", remotePath)
	}
	return nil
}

// createDirectories recursively creates directories on the SMB share
func createDirectories(share *smb2.Share, dirPath string) error {
	components := strings.Split(dirPath, "/")
	currentPath := ""

	for _, component := range components {
		if component == "" {
			continue
		}
		if currentPath != "" {
			currentPath += "/"
		}
		currentPath += component

		info, err := share.Stat(currentPath)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s exists but is not a directory", currentPath)
			}
		} else {
			if err = share.MkdirAll(currentPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %v", currentPath, err)
			}
		}
	}
	return nil
}

// listFiles lists files on an SMB share using a glob pattern
func listFiles(server, path string) ([]string, error) {
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return nil, err
	}
	defer session.Logoff()

	shareName, filePath, err := parseSharePath(path)
	if err != nil {
		return nil, err
	}

	share, err := session.Mount(shareName)
	if err != nil {
		return nil, fmt.Errorf("failed to mount share %s: %v", shareName, err)
	}
	defer share.Umount()

	files, err := share.Glob(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %v", err)
	}

	files = u.SliceMap(files, func(s string) *string {
		s1 := strings.ReplaceAll(s, `\`, `/`)
		return &s1
	})
	return files, nil
}

// cleanOldFiles finds files older than a certain number of days and deletes them (or just lists them if dry-run)
func cleanOldFiles(server, path string, days int, dryRun bool, verbose bool) error {
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return err
	}
	defer session.Logoff()

	shareName, filePath, err := parseSharePath(path)
	if err != nil {
		return err
	}

	share, err := session.Mount(shareName)
	if err != nil {
		return fmt.Errorf("failed to mount share %s: %v", shareName, err)
	}
	defer share.Umount()

	files, err := share.Glob(filePath)
	if err != nil {
		return fmt.Errorf("failed to list files for cleaning: %v", err)
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -days)
	count := 0

	for _, f := range files {
		// Normalize path
		fPath := strings.ReplaceAll(f, `\`, `/`)

		info, err := share.Stat(fPath)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Could not stat %s: %v\n", fPath, err)
			}
			continue
		}

		if info.ModTime().Before(cutoff) {
			if dryRun {
				fmt.Fprintf(os.Stdout, "[DRY-RUN] Would remove: %s (Modified: %v)\n", fPath, info.ModTime())
				count++
			} else {
				if err := share.Remove(fPath); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", fPath, err)
				} else {
					if verbose {
						fmt.Fprintf(os.Stderr, "Removed: %s\n", fPath)
					}
					count++
				}
			}
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Clean operation completed. Files processed: %d\n", count)
	} else if dryRun {
		fmt.Fprintf(os.Stdout, "Dry run completed. Found %d files to remove.\n", count)
	}

	return nil
}
