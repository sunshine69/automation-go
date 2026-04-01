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

	var (
		uploadSource string
		uploadDest   string
	)
	uploadCmd.StringVar(&uploadSource, "src", "-", "Source file (use - for stdin)")
	uploadCmd.StringVar(&uploadDest, "dest", "", "Destination path on SMB share (e.g. /sharename/path/to/file.txt)")

	var (
		downloadSource string
		downloadDest   string
	)
	downloadCmd.StringVar(&downloadSource, "src", "", "Source file on SMB share")
	downloadCmd.StringVar(&downloadDest, "dest", "-", "Destination local file (use - for stdout)")

	var (
		mvSource string
		mvDest   string
	)
	mvCmd.StringVar(&mvSource, "src", "", "Source file on SMB share")
	mvCmd.StringVar(&mvDest, "dest", "", "Destination path on SMB share")

	var rmPath, lsPath string
	rmCmd.StringVar(&rmPath, "path", "", "File to remove on SMB share")
	lsCmd.StringVar(&lsPath, "path", "", "Path/glob pattern to list on SMB share")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Split args into main flags and subcommand args
	mainArgs := []string{}
	subCmdPos := len(os.Args) // default: no subcommand found
	for i, arg := range os.Args[1:] {
		if arg == "upload" || arg == "download" || arg == "mv" || arg == "rm" || arg == "ls" {
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
		fmt.Fprintln(os.Stderr, "Error: expected subcommand: upload, download, mv, rm, ls")
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
		err = removeFile(serverFlag, rmPath, verboseFlag)

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
  upload   -src <local_file|-stdin> -dest </share/path>
  download -src </share/path> -dest <local_file|-stdout>
  mv       -src </share/path> -dest </share/path>
  rm       -path </share/path>
  ls       -path </share/glob_pattern>

Examples:
  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    upload -src ./file.txt -dest /sharename/tmp/file.txt

  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    download -src /sharename/tmp/file.txt -dest -

  go-smb-tool -server bnefs:445 -login 'DOMAIN\user' -password "$pass" -domain DOMAIN \
    ls -path /sharename/tmp/*.txt`)
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

// upload uploads a file to an SMB share
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

	if filePath != "" {
		dirPath := filepath.Dir(filePath)
		if dirPath != "." {
			if err = createDirectories(share, dirPath); err != nil {
				return fmt.Errorf("failed to create directories: %v", err)
			}
		}
	}

	destFile, err := share.Create(filePath)
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
		fmt.Fprintf(os.Stderr, "Uploaded %d bytes to %s\n", bytesWritten, destPath)
	}
	return nil
}

// download downloads a file from an SMB share
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

	srcFile, err := share.Open(filePath)
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
		fmt.Fprintf(os.Stderr, "Downloaded %d bytes from %s\n", bytesRead, srcPath)
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
		if err = removeFile(server, srcPath, false); err != nil {
			return fmt.Errorf("warning: failed to remove source file: %v", err)
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Moved %s to %s\n", srcPath, destPath)
	}
	return nil
}

// removeFile deletes a file from an SMB share
func removeFile(server, path string, verbose bool) error {
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

	if err = share.Remove(filePath); err != nil {
		return fmt.Errorf("failed to remove file: %v", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Removed %s\n", path)
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
