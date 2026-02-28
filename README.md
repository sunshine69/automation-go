## What is

A collection of tools, functions etc helpfull to write go automation program.

My initial thing is to port ansible to go but then I do not think it is needed. Instead I would like to shape a pattern and a frequently uses functions to write automation code using golang. Keep it simple.

In plays folder there are several commands which I found usefull.

### cred-detect
a password scan tools.

build:
```
env CGO_ENABLED=0 go build -trimpath -ldflags="-X main.version=1.0 -X main.buildTime=(date +%Y%m%d%H%M) 
-extldflags=-static -w -s" --tags "osusergo,netgo,sqlite_stat4,sqlite_foreign_keys,sqlite_json" -o cred-detect plays/cred-detect/main.go

# You can test the binary but remember to use the options `--words-file` pointing to the words file, you can add more words into it to teach the cli know the words, so it wont report it as real password. The file is in plays/cred-detect/data/words.txt. aFter done, compress this file using command `lzma` and then append it to the binary

rice append --exec cred-detect -i plays/cred-detect/main.go

```

The final binary ld have the dta file built in, if user does not provide options `--words-file` then it will use its default.

Run `cred-detect -h` for complete help.

### go-smb-tool

Allow to handle basic smb operations

### lineinfile

Simulate ansible lineinfile but include some powerful feature to allow text manipulations and greping

### pass-strength

Check password strength

### vault.

A command line to encrypt. Intended to use with the golang inventory parser system. See the file `lib/inventory_test.go` for the example of what it is about and how to use it. See the project https://github.com/sunshine69/go-automation as an example of its usages.
