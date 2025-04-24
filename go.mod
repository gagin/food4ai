module github.com/gagin/codecat // Replace with your actual repo path if different

go 1.21 // Or your target Go version (e.g., 1.22)

// Indirect dependencies might be added automatically by 'go mod tidy'
// Example (versions might differ):
// require (
//	github.com/davecgh/go-spew v1.1.1 // indirect
//	github.com/pmezard/go-difflib v1.0.0 // indirect
//	gopkg.in/yaml.v3 v3.0.1 // indirect
// )

require (
	github.com/BurntSushi/toml v1.5.0
	github.com/boyter/gocodewalker v1.4.0
	github.com/spf13/pflag v1.0.6
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
