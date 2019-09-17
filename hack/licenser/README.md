# Licenser

This tool is used to generate file `NOTICE.txt` which contains certain notices of software components included in ECK.

## Usage

Tool is designed to work with [Go modules](https://github.com/golang/go/wiki/Modules) for managing dependencies, i.e. it requires Go 1.11+ to work. It expects to have all dependencies downloaded in `vendor` directory and file `vendor/modules.txt` exists. It can be achieved by running `go mod vendor`     

To run it use:
```go
go mod vendor 
cd hack/licenser
go run main.go -d path/to/repo
```