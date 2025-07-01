// for golang code gen seems this one generate flawless code (at least no error). local qwen2.5-coder 32b seems to be good but need to give example for the go-smb2 example. ChatGPT is wrong about the smb2.Dial as well - second edit would fix it
// https://claude.ai/ it even does not need the smb2 code example
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-jwt/jwt"
	"github.com/hirochachacha/go-smb2"
	u "github.com/sunshine69/golang-tools/utils"
)

// Config represents the structure of our config file
type Config struct {
	Users map[string]UserConfig
}

// UserConfig holds user-specific configuration
type UserConfig struct {
	JWTSecret    string
	AllowedPaths []string
	Domain       string
}

// Global flags
var (
	tokenFlag   string
	configFlag  string
	serverFlag  string
	loginUser   string
	loginPass   string
	smbDomain   string
	verboseFlag bool
)

func main() {
	// Main flagset
	mainFlagSet := flag.NewFlagSet("smbutil", flag.ExitOnError)
	mainFlagSet.StringVar(&tokenFlag, "tok", "", "JWT token for authentication")
	mainFlagSet.StringVar(&configFlag, "config", "config.json", `Path to config file. This can be passed by seeting the env var GO_SMB_TOOL_CONFIG.
	Example config.json:
	{
    	"someuser_name": {
			"Domain": "smb.domain",
			"JWTSecret": "secret",
        	"AllowedPaths": ["/<sharename>/tmp/"]
    	}
	}
	Example command: (assume you use jwt tool to generate $token using the secret above)
	go-smb-tool-linux-amd64 -login 'AUSIGBNE.ANSSVC' -password "$smbpass" -tok $token -server 'bnefs:445' upload -dest '/ansible-awx/tmp/test.tx
t' -src go.mod

	available commands: (using with option -src <SOURCE_FILE> or -dest <DESTINATION_FILE> Replace file with dash - to read/write to stdin|stdout)
	  - upload   - To upload to a remote SMB share from a local sile system or stdin
	  - download - Download from the remote smb share to local file system or stdin
	  - mv       - move/rename file from smb remote share
	  - rm       - delete file from smb remote share
	  - ls <path/glob-filename-pattern> - List files
	`)
	mainFlagSet.StringVar(&serverFlag, "server", "", "SMB server address (hostname:port) Reminder, default port for smb is 445")
	mainFlagSet.StringVar(&loginUser, "login", "", "Login user to the smb share")
	mainFlagSet.StringVar(&loginPass, "password", "", "Login password")
	mainFlagSet.StringVar(&smbDomain, "domain", "", "Domain")
	mainFlagSet.BoolVar(&verboseFlag, "verbose", false, "Enable verbose output")

	// Subcommands
	uploadCmd := flag.NewFlagSet("upload", flag.ExitOnError)
	downloadCmd := flag.NewFlagSet("download", flag.ExitOnError)
	mvCmd := flag.NewFlagSet("mv", flag.ExitOnError)
	rmCmd := flag.NewFlagSet("rm", flag.ExitOnError)
	lsCmd := flag.NewFlagSet("ls", flag.ExitOnError)

	// Upload flags
	var (
		uploadSource string
		uploadDest   string
	)
	uploadCmd.StringVar(&uploadSource, "src", "-", "Source file (use - for stdin)")
	uploadCmd.StringVar(&uploadDest, "dest", "", "Destination path on SMB share")

	// Download flags
	var (
		downloadSource string
		downloadDest   string
	)
	downloadCmd.StringVar(&downloadSource, "src", "", "Source file on SMB share")
	downloadCmd.StringVar(&downloadDest, "dest", "-", "Destination local file (use - for stdout)")

	// Move flags
	var (
		mvSource string
		mvDest   string
	)
	mvCmd.StringVar(&mvSource, "src", "", "Source file on SMB share")
	mvCmd.StringVar(&mvDest, "dest", "", "Destination path on SMB share")

	// Remove flags
	var (
		rmPath, lsPath string
	)
	rmCmd.StringVar(&rmPath, "path", "", "File to remove on SMB share")
	lsCmd.StringVar(&lsPath, "path", "", "File to remove on SMB share")

	// Check if we have enough arguments
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Expected subcommand: upload, download, mv, rm, ls")
		os.Exit(1)
	}

	// Parse the main flags first (must be before the subcommand)
	mainArgs := []string{}
	subCmdPos := 1
	for i, arg := range os.Args[1:] {
		if arg == "upload" || arg == "download" || arg == "mv" || arg == "rm" || arg == "ls" {
			subCmdPos = i + 1
			break
		}
		mainArgs = append(mainArgs, arg)
	}

	mainFlagSet.Parse(mainArgs)

	if loginPass == "" {
		loginPass = os.Getenv("SMB_PASSWORD")
	}

	// Load configuration
	config, err := loadConfig(configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Parse JWT token to get username
	username, err := parseJWTToken(tokenFlag, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "JWT token error: %v\n", err)
		os.Exit(1)
	}

	// Get user config
	userConfig, exists := config.Users[username]
	if !exists {
		fmt.Fprintf(os.Stderr, "User %s not found in config\n", username)
		os.Exit(1)
	}

	if smbDomain == "" {
		if userConfig.Domain == "" {
			panic("domain must be provided in commandline or configfile")
		}
		smbDomain = userConfig.Domain
	}

	if verboseFlag {
		fmt.Fprintf(os.Stderr, "Authenticated as: %s\n", username)
	}

	// Execute the appropriate subcommand
	switch os.Args[subCmdPos] {
	case "upload":
		uploadCmd.Parse(os.Args[subCmdPos+1:])
		if uploadDest == "" {
			fmt.Fprintln(os.Stderr, "Error: Destination path is required for upload")
			uploadCmd.PrintDefaults()
			os.Exit(1)
		}

		// Validate path against allowed paths
		if !isPathAllowed(uploadDest, userConfig.AllowedPaths) {
			fmt.Fprintf(os.Stderr, "Access denied: Not allowed to upload to %s\n", uploadDest)
			os.Exit(1)
		}

		err = upload(serverFlag, uploadSource, uploadDest, verboseFlag)

	case "download":
		downloadCmd.Parse(os.Args[subCmdPos+1:])
		if downloadSource == "" {
			fmt.Fprintln(os.Stderr, "Error: Source path is required for download")
			downloadCmd.PrintDefaults()
			os.Exit(1)
		}

		// Validate path against allowed paths
		if !isPathAllowed(downloadSource, userConfig.AllowedPaths) {
			fmt.Fprintf(os.Stderr, "Access denied: Not allowed to download from %s\n", downloadSource)
			os.Exit(1)
		}

		err = download(serverFlag, downloadSource, downloadDest, verboseFlag)

	case "mv":
		mvCmd.Parse(os.Args[subCmdPos+1:])
		if mvSource == "" || mvDest == "" {
			fmt.Fprintln(os.Stderr, "Error: Source and destination paths are required for move")
			mvCmd.PrintDefaults()
			os.Exit(1)
		}

		// Validate both paths against allowed paths
		if !isPathAllowed(mvSource, userConfig.AllowedPaths) || !isPathAllowed(mvDest, userConfig.AllowedPaths) {
			fmt.Fprintf(os.Stderr, "Access denied: Not allowed to move between these locations\n")
			os.Exit(1)
		}

		err = moveFile(serverFlag, mvSource, mvDest, verboseFlag)

	case "rm":
		rmCmd.Parse(os.Args[subCmdPos+1:])
		if rmPath == "" {
			fmt.Fprintln(os.Stderr, "Error: Path is required for remove")
			rmCmd.PrintDefaults()
			os.Exit(1)
		}

		// Validate path against allowed paths
		if !isPathAllowed(rmPath, userConfig.AllowedPaths) {
			fmt.Fprintf(os.Stderr, "Access denied: Not allowed to delete %s\n", rmPath)
			os.Exit(1)
		}

		err = removeFile(serverFlag, rmPath, verboseFlag)
	case "ls":
		lsCmd.Parse(os.Args[subCmdPos+1:])
		if lsPath == "" {
			fmt.Fprintln(os.Stderr, "Error: Path is required for ls command")
			rmCmd.PrintDefaults()
			os.Exit(1)
		}

		// Validate path against allowed paths
		if !isPathAllowed(lsPath, userConfig.AllowedPaths) {
			fmt.Fprintf(os.Stderr, "Access denied: Not allowed to ls %s\n", lsPath)
			os.Exit(1)
		}
		files := u.Must(listFiles(serverFlag, lsPath))
		println(u.JsonDump(files, ""))
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", os.Args[subCmdPos])
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Operation failed: %v\n", err)
		os.Exit(1)
	}
}

// loadConfig loads the configuration from a file
func loadConfig(configPath string) (Config, error) {
	configString := os.Getenv("GO_SMB_TOOL_CONFIG")
	if configString == "" {
		fmt.Fprintln(os.Stderr, "No env var GO_SMB_TOOL_CONFIG available. Trying to load from config file "+configPath)
		configString = string(u.Must(os.ReadFile(configPath)))
	}
	users := map[string]UserConfig{}
	u.Must("", json.Unmarshal([]byte(configString), &users))

	config := Config{
		Users: users,
	}
	return config, nil
}

// parseJWTToken parses a JWT token and returns the subject (username)
func parseJWTToken(tokenString string, config Config) (string, error) {
	// First, parse the token without verifying to get the username
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims in token")
	}

	subject, ok := claims["sub"].(string)
	if !ok {
		return "", fmt.Errorf("subject not found in token")
	}

	// Now that we have the username, get their secret and validate the token
	userConfig, exists := config.Users[subject]
	if !exists {
		return "", fmt.Errorf("user %s not found in config", subject)
	}

	// Verify the token with the user's secret
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = userConfig.JWTSecret
	}
	parsedToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the algorithm
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to validate token: %v", err)
	}

	if !parsedToken.Valid {
		return "", fmt.Errorf("invalid token")
	}

	return subject, nil
}

// isPathAllowed checks if a path is allowed for the user
func isPathAllowed(path string, allowedPaths []string) bool {
	for _, allowedPath := range allowedPaths {
		if strings.HasPrefix(path, allowedPath) {
			return true
		}
	}
	return false
}

// connectToSMB establishes a connection to the SMB server
func connectToSMB(server, username, password, smbDomain string) (*smb2.Session, error) {
	// In a real application, you would get the password from a secure source
	// For this example, we'll use a placeholder

	conn, err := net.Dial("tcp", server)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %v", err)
	}

	dialer := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     username,
			Password: password,
			Domain:   smbDomain,
		},
	}

	session, err := dialer.Dial(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SMB authentication failed: %v", err)
	}

	return session, nil
}

// parseSharePath splits a path into share name and file path
func parseSharePath(path string) (string, string, error) {
	if !strings.HasPrefix(path, "/") {
		return "", "", fmt.Errorf("path must start with /")
	}

	// Remove leading slash
	path = path[1:]

	// Split at the first slash
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
	// Connect to SMB
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return err
	}
	defer session.Logoff()

	// Parse the destination path
	shareName, filePath, err := parseSharePath(destPath)
	if err != nil {
		return err
	}

	// Open the share
	share, err := session.Mount(shareName)
	if err != nil {
		return fmt.Errorf("failed to mount share %s: %v", shareName, err)
	}
	defer share.Umount()

	// Create parent directories if they don't exist
	if filePath != "" {
		dirPath := filepath.Dir(filePath)
		if dirPath != "." {
			err = createDirectories(share, dirPath)
			if err != nil {
				return fmt.Errorf("failed to create directories: %v", err)
			}
		}
	}

	// Create the destination file
	destFile, err := share.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer destFile.Close()

	// Open the source file or stdin
	var srcFile io.Reader
	if srcPath == "-" {
		srcFile = bufio.NewReader(os.Stdin)
		if verbose {
			fmt.Fprintln(os.Stderr, "Reading from stdin...")
		}
	} else {
		file, err := os.Open(srcPath)
		if err != nil {
			return fmt.Errorf("failed to open source file: %v", err)
		}
		defer file.Close()
		srcFile = file
		if verbose {
			fmt.Fprintf(os.Stderr, "Reading from file: %s\n", srcPath)
		}
	}

	// Copy the data
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
	// Connect to SMB
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return err
	}
	defer session.Logoff()

	// Parse the source path
	shareName, filePath, err := parseSharePath(srcPath)
	if err != nil {
		return err
	}

	// Open the share
	share, err := session.Mount(shareName)
	if err != nil {
		return fmt.Errorf("failed to mount share %s: %v", shareName, err)
	}
	defer share.Umount()

	// Open the source file
	srcFile, err := share.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer srcFile.Close()

	// Open the destination file or stdout
	var destFile io.Writer
	if destPath == "-" {
		destFile = os.Stdout
		if verbose {
			fmt.Fprintln(os.Stderr, "Writing to stdout...")
		}
	} else {
		file, err := os.Create(destPath)
		if err != nil {
			return fmt.Errorf("failed to create destination file: %v", err)
		}
		defer file.Close()
		destFile = file
		if verbose {
			fmt.Printf("Writing to file: %s\n", destPath)
		}
	}

	// Copy the data
	bytesRead, err := io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy data: %v", err)
	}

	if verbose {
		if destPath != "-" {
			fmt.Printf("Downloaded %d bytes from %s\n", bytesRead, srcPath)
		} else {
			fmt.Fprintf(os.Stderr, "Downloaded %d bytes from %s\n", bytesRead, srcPath)
		}
	}

	return nil
}

// moveFile moves a file within SMB shares
func moveFile(server, srcPath, destPath string, verbose bool) error {
	// Connect to SMB
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return err
	}
	defer session.Logoff()

	// Parse the source path
	srcShareName, srcFilePath, err := parseSharePath(srcPath)
	if err != nil {
		return err
	}

	// Parse the destination path
	destShareName, destFilePath, err := parseSharePath(destPath)
	if err != nil {
		return err
	}

	// Check if this is a move within the same share
	if srcShareName == destShareName {
		// Open the share
		share, err := session.Mount(srcShareName)
		if err != nil {
			return fmt.Errorf("failed to mount share %s: %v", srcShareName, err)
		}
		defer share.Umount()

		// Create parent directories if they don't exist
		dirPath := filepath.Dir(destFilePath)
		if dirPath != "." {
			err = createDirectories(share, dirPath)
			if err != nil {
				return fmt.Errorf("failed to create directories: %v", err)
			}
		}

		// Rename the file
		err = share.Rename(srcFilePath, destFilePath)
		if err != nil {
			return fmt.Errorf("failed to rename file: %v", err)
		}

		if verbose {
			fmt.Printf("Moved %s to %s\n", srcPath, destPath)
		}
	} else {
		// Moving between different shares, we need to copy and delete
		// First, download the file to a temporary location
		tempFile, err := os.CreateTemp("", "smb-move-*")
		if err != nil {
			return fmt.Errorf("failed to create temporary file: %v", err)
		}
		tempFileName := tempFile.Name()
		tempFile.Close()
		defer os.Remove(tempFileName)

		// Download the source file
		err = download(server, srcPath, tempFileName, false)
		if err != nil {
			return fmt.Errorf("failed to download source file: %v", err)
		}

		// Upload the file to the destination
		err = upload(server, tempFileName, destPath, false)
		if err != nil {
			return fmt.Errorf("failed to upload to destination: %v", err)
		}

		// Remove the source file
		err = removeFile(server, srcPath, false)
		if err != nil {
			return fmt.Errorf("warning: failed to remove source file: %v", err)
		}

		if verbose {
			fmt.Printf("Moved %s to %s\n", srcPath, destPath)
		}
	}

	return nil
}

// removeFile deletes a file from an SMB share
func removeFile(server, path string, verbose bool) error {
	// Connect to SMB
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return err
	}
	defer session.Logoff()

	// Parse the path
	shareName, filePath, err := parseSharePath(path)
	if err != nil {
		return err
	}

	// Open the share
	share, err := session.Mount(shareName)
	if err != nil {
		return fmt.Errorf("failed to mount share %s: %v", shareName, err)
	}
	defer share.Umount()

	// Remove the file
	err = share.Remove(filePath)
	if err != nil {
		return fmt.Errorf("failed to remove file: %v", err)
	}

	if verbose {
		fmt.Printf("Removed %s\n", path)
	}

	return nil
}

// createDirectories recursively creates directories
func createDirectories(share *smb2.Share, dirPath string) error {
	// Split the path into components
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

		// Check if directory exists
		info, err := share.Stat(currentPath)
		if err == nil {
			// Path exists, check if it's a directory
			if !info.IsDir() {
				return fmt.Errorf("%s exists but is not a directory", currentPath)
			}
		} else {
			// Create the directory
			err = share.MkdirAll(currentPath, 0755)
			if err != nil {
				return fmt.Errorf("failed to create directory %s: %v", currentPath, err)
			}
		}
	}

	return nil
}

// listFiles list files from an SMB share using a glob file path patterm
func listFiles(server, path string) (files []string, err1 error) {
	// Connect to SMB
	session, err := connectToSMB(server, loginUser, loginPass, smbDomain)
	if err != nil {
		return files, err
	}
	defer session.Logoff()

	// Parse the path
	shareName, filePath, err := parseSharePath(path)
	if err != nil {
		return files, err
	}

	// Open the share
	share, err := session.Mount(shareName)
	if err != nil {
		return files, fmt.Errorf("failed to mount share %s: %v", shareName, err)
	}
	defer share.Umount()

	// Remove the file
	files, err = share.Glob(filePath)
	if err != nil {
		return files, fmt.Errorf("failed to remove file: %v", err)
	}
	files = u.SliceMap(files, func(s string) *string { s1 := strings.ReplaceAll(s, `\`, `/`); return &s1 })
	return files, nil
}
