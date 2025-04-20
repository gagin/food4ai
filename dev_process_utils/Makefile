COMMIT_MESSAGE ?= $(strip Updated changes to $(shell git describe --tags --always))

# Build the codecat binary
build:
    go build -o codecat .

# Increment version in main.go and stage it
increment-version:
    go run dev_process_utils/increment_version.go
    git add main.go

# Commit staged changes with dynamic message
commit: increment-version
    git add --update
    git commit -m "$(COMMIT_MESSAGE)"

# Default target: run commit
.PHONY: default commit build increment-version
default: commit

# Catch-all for undefined targets
%:
    @: