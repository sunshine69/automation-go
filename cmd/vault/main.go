package main

import (
	"fmt"
	u "github.com/sunshine69/golang-tools/utils"
	"os"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] == "-h" {
		fmt.Fprintf(os.Stderr, "Usage: govault <cmd> <data> where <cmd> [password] can be: encrypt|decrypt. <data> is the string. If [password] not provided, it will read the env var VAULT_PASSWORD")
		os.Exit(1)
	}
	password := u.Ternary(len(os.Args) == 4, os.Args[3], os.Getenv("VAULT_PASSWORD"))
	switch os.Args[1] {
	case "encrypt":
		fmt.Fprintln(os.Stdout, u.Must(u.Encrypt(os.Args[2], password, u.DefaultEncryptionConfig())))
	case "decrypt":
		fmt.Fprintln(os.Stdout, u.Must(u.Decrypt(os.Args[2], password, u.DefaultEncryptionConfig())))
	default:
		fmt.Fprintln(os.Stderr, "no such command "+os.Args[1]+"\n Run with option -h for help")
	}
}
