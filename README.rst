codecat
=======
**Version:** 0.2.0

A command-line tool to concatenate source code files into a single output,
formatted for easy consumption by Large Language Models (LLMs) or other AI
analysis tools.

Purpose
-------

When working with AI models for code analysis, refactoring, or question
answering, it's often necessary to provide the model with context from your
codebase. Manually copying and pasting files is tedious and error-prone.

``codecat`` automates this process by scanning a target directory, filtering
files based on extensions, exclusion patterns, and ``.gitignore`` rules,
and concatenating their contents into a single output stream or file. Each
file's content is clearly delimited with markers indicating the filename.
A summary of included files, sizes, and any errors is printed separately.

Installation
------------
Assuming you have Go installed and your GOPATH/GOBIN is set up:

.. code-block:: bash

    # Build from source:
    git clone https://github.com/gagin/codecat.git
    cd codecat
    go build -o codecat .

Move the resulting ``codecat`` executable to a directory in your system's PATH
(e.g., ``/usr/local/bin`` or ``~/.local/bin``) to run it from anywhere.

Usage
-----

The tool operates in two main modes:

**Mode 1: Positional Argument (Simple)**

Scan a single directory using default or config settings. Cannot be mixed with most other flags.

.. code-block:: bash

    codecat [target_directory]

* ``target_directory``: The directory to scan. Defaults to '.' if omitted.

**Mode 2: Flags Only (Advanced)**

Use flags for specific control (directory, extensions, files, excludes, output). No positional arguments allowed in this mode.

.. code-block:: bash

    codecat [flags]

**Flags:**

* **-d, --directory** *path*
    Target directory to scan (use this *or* a positional argument, not both). Defaults to the current directory (``.``).

* **-e, --extensions** *ext1,ext2,...*
    Comma-separated list of file extensions (without leading dot) to include (requires flags mode). Can be repeated. Overrides config.

* **-f, --files** *path1,path2,...*
    Comma-separated list of specific file paths to include manually (requires flags mode). Bypasses filters and ``.gitignore``. Paths can be absolute or relative.

* **-x, --exclude** *pattern1,pattern2,...*
    Comma-separated list of glob patterns to exclude (requires flags mode). Matched against paths relative to the target directory. Can be repeated. Overrides config.

* **--no-gitignore**
    Disable processing of ``.gitignore`` files found in the target directory (requires flags mode). Overrides config setting. By default, ``.gitignore`` at the root is processed.

* **-o, --output** *path*
    Write concatenated code to *path* instead of stdout. Summary/logs go to stdout. If omitted, code goes to stdout and summary/logs go to stderr.

* **--config** *path*
    Path to a custom configuration file. Defaults to ``~/.config/codecat/config.toml``.

* **--loglevel** *(debug|info|warn|error)*
    Set logging verbosity. Defaults to ``info``. Logs always go to stderr.

* **-h, --help**
    Show this help message and exit.

* **-v, --version**
    Show version information and exit. (Note: Version flag not explicitly implemented in provided code, add if needed).


Configuration
-------------

``codecat`` can be configured using a TOML file, typically located at
``~/.config/codecat/config.toml`` (changeable with ``--config``).

Example (`config.toml`):

.. code-block:: toml

    # The introductory text placed at the very beginning of the code output.
    header_text = "Codebase for analysis:"

    # List of file extensions (without leading dot) to include by default.
    # Overridden by -e flag.
    include_extensions = [
      "go", "mod", "sum", # Go project files
      "py", "ipynb",      # Python
      "js", "ts", "jsx", "tsx", "html", "css", "json", "yaml", "yml", # Web dev
      "md", "rst", "txt", # Documentation/Text
      "sh", "bash",       # Shell scripts
      "toml",             # Config files
      "dockerfile", "Dockerfile"
    ]

    # List of glob patterns to exclude by default. Applied relative to target dir.
    # Overridden by -x flag. Manually added files (-f) are NOT affected.
    exclude_patterns = [
      "*.log",
      "dist/*",
      "build/*",
      "node_modules/*",
      "venv/*",
      ".git/*", # Usually handled by gitignore too
      "__pycache__/*",
      ".pytest_cache/*",
      "*.pyc",
      "*.pyo",
      "*.swp",
      "*.bak",
      ".DS_Store"
    ]

    # The marker used to delimit file sections in the code output.
    comment_marker = "---" # Example: --- path/file.ext

    # Whether to respect .gitignore file at the root of the target directory by default.
    # Overridden by --no-gitignore flag.
    use_gitignore = true


Output Format
-------------

**Concatenated Code:**
* Sent to stdout by default, or to the file specified by ``-o``.
* Starts with ``header_text`` from config (if any).
* Each included file's content is wrapped by marker lines indicating the path (relative to the target directory if possible):
    .. code-block:: text

        Codebase for analysis:

        --- file1.go
        package main
        //...
        ---

        --- internal/helper.go
        package internal
        // ...
        ---

**Summary & Logs:**
* Sent to stderr by default, or to stdout if ``-o`` is used.
* Includes informational messages during processing (INFO level and above).
* Ends with a summary section detailing the operation results:
    .. code-block:: text

        --- Summary ---
        Included 2 files (1.5 KiB total) from '/path/to/project':
        └── file1.go (1.1 KiB)
        └── internal/
            └── helper.go (450 B)

        Empty files found (1):
        - empty.txt

        Errors encountered (1):
        - unreadable.txt: open /path/to/project/unreadable.txt: permission denied
        ---------------

* Manually included files are marked with `[M]` in the tree.


Example Usage
-------------

Scan directory `src` using defaults, sending code to stdout, summary to stderr:
.. code-block:: bash
    codecat src

Scan current directory, include only Go files, exclude vendor dir, write code to `codebase.txt`, summary to stdout:
.. code-block:: bash
    codecat -e go -x "vendor/*" -o codebase.txt

Include specific file and all `.yaml` files from `conf` directory, sending code to stdout, summary to stderr:
.. code-block:: bash
    codecat -f config/main.toml -d conf -e yaml

Process only a specific manual file, sending code to `manual_only.txt`, summary to stdout:
.. code-block:: bash
    codecat -f /path/to/important/file.py -o manual_only.txt


Version History
---------------
- Apr 20, 2025: Renaming `food4ai` to `codecat` — short, clear, and reflects code concatenation for LLMs. Seems unused.

To-Do and Known Problems
------------------------
- Follows `most specific` approach for .gitignore instead of standard `first-seen`
- main_test fails