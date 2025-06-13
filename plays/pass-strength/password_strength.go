package main

import (
	"fmt"
	"os"

	lib "github.com/sunshine69/automation-go/lib"
	u "github.com/sunshine69/golang-tools/utils"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run password_strength.go <password1> <password2> ...")
		return
	}

	passwords := os.Args[1:]
	result := []map[string]any{}

	for _, password := range passwords {
		strength, entropyBit, err := lib.CheckPasswordStrength(password)
		if err != nil {
			result = append(result, map[string]any{"Password": password, "Strength": strength, "Entropy": entropyBit, "Error": err.Error()})
		} else {
			result = append(result, map[string]any{"Password": password, "Strength": strength, "Entropy": entropyBit})
		}
	}
	fmt.Println(u.JsonDump(result, "  "))
}
