codecat
=======
**Version:** 0.3.0

A command-line tool to concatenate source code files into a single output,
formatted for easy consumption by Large Language Models (LLMs) or other AI
analysis tools. Now uses ``gocodewalker`` for improved, Git-compatible
``.gitignore`` handling.

.. image:: https://tokei.rs/b1/github/gagin/codecat
   :alt: Lines of Code
   :target: https://github.com/gagin/codecat

Purpose
-------

When working with AI models for code analysis, refactoring, or question
answering, it's often necessary to provide the model with context from your
codebase. Manually copying and pasting files is tedious and error-prone.

``codecat`` automates this process by scanning a target directory, filtering
files based on extensions, exclusion patterns (from config and command line),
and recursively found ``.gitignore``/``.ignore`` files (Git-compatible behavior),
and concatenating their contents into a single output stream or file. Each
file's content is clearly delimited with markers indicating the filename.
A summary of included files, sizes, and any errors is printed separately.

See `usecases.jsonc <./usecases.jsonc>`_ for detailed usage scenarios.
See `TODO.rst <./TODO.rst>`_ for planned features and improvements.


Installation
------------
Assuming you have Go installed and your GOPATH/GOBIN is set up:

.. code-block:: bash

    # Build from source:
    # Note: The main code is now in cmd/codecat/
    git clone https://github.com/gagin/codecat.git
    cd codecat
    go build -o codecat ./cmd/codecat

Move the resulting ``codecat`` executable to a directory in your system's PATH
(e.g., ``/usr/local/bin`` or ``~/.local/bin``) to run it from anywhere.


Usage
-----

The tool operates in two main modes:

**Mode 1: Positional Argument (Simple)**

Scan a single directory using default or config settings. Cannot be mixed with most other flags.

.. code-block:: bash

    codecat [target_directory]

* ``target_directory``: The directory to scan. Defaults to '.' if omitted. Respects ``.gitignore``/``.ignore`` files recursively by default.

**Mode 2: Flags Only (Advanced)**

Use flags for specific control (directory, extensions, files, excludes, output, etc.). No positional arguments allowed in this mode.

.. code-block:: bash

    codecat [flags]

**Flags:**

* **-d, --directory** *path*
    Target directory to scan (use this *or* a positional argument, not both). Defaults to the current directory (``.``).

* **-e, --extensions** *ext1,ext2,...*
    Comma-separated list of file extensions (without leading dot) to include. Can be repeated. Overrides config's `include_extensions`.

* **-f, --files** *path1,path2,...*
    Comma-separated list of specific file paths to include manually. Bypasses filters and ``.gitignore``. Paths can be absolute or relative.

* **-x, --exclude** *pattern1,pattern2,...*
    Comma-separated list of glob patterns (standard Go `filepath.Match` syntax) to exclude. Matched against paths relative to the target directory. Can be repeated. Adds to patterns defined in config's `exclude_patterns`. Use `dir/` to exclude a directory and its contents.

* **--no-gitignore**
    Disable processing of ``.gitignore`` and ``.ignore`` files found recursively. By default (without this flag), Git-compatible recursive ignore processing is enabled. Overrides config's `use_gitignore`.

* **-n, --no-scan**
    Skip directory scanning entirely. Only processes files specified manually via `-f`. If no `-f` files are given, output will be empty.

* **-o, --output** *path*
    Write concatenated code to *path* instead of stdout. Summary/logs go to stdout. If omitted, code goes to stdout and summary/logs go to stderr.

* **--config** *path*
    Path to a custom configuration file. Defaults to ``~/.config/codecat/config.toml``.

* **--loglevel** *(debug|info|warn|error)*
    Set logging verbosity. Defaults to ``info``. Logs always go to stderr (unless `-o` is used, then logs go to stdout).

* **-h, --help**
    Show help message and exit.

* **-v, --version**
    Show version information and exit.


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
    # Patterns from the -x flag are ADDED to this list.
    # Manually added files (-f) are NOT affected by these.
    exclude_patterns = [
      "*.log",
      "dist/", # Trailing slash excludes directory and contents
      "build/",
      "node_modules/",
      "venv/",
      ".git/", # Also handled by gitignore if enabled
      "__pycache__/",
      ".pytest_cache/",
      "*.pyc",
      "*.pyo",
      "*.swp",
      "*.bak",
      ".DS_Store"
    ]

    # The marker used to delimit file sections in the code output.
    comment_marker = "---" # Example: --- path/file.ext

    # Whether to respect .gitignore/.ignore files recursively (Git-compatible).
    # Set to false to disable ignore file processing by default.
    # Overridden by the --no-gitignore command-line flag.
    use_gitignore = true

    # --- Future Options ---
    # include_file_list_in_output = false
    # include_empty_files_in_output = false


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
        /path/to/project/
        ├── file1.go (1.1 KiB)
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

Scan current directory using defaults (respects .gitignore recursively):
.. code-block:: bash
    codecat > output.txt

Scan current directory, disable .gitignore, exclude tests/ dir, include only .go files, write to file:
.. code-block:: bash
    codecat --no-gitignore -x tests/ -e go -o codebase.go.txt

Process only manually specified files, skipping directory scan:
.. code-block:: bash
    codecat -n -f cmd/codecat/main.go -f cmd/codecat/walk.go -o core_logic.go

Scan `src` dir, adding `*.tmp` to configured excludes, writing code to stdout:
.. code-block:: bash
    codecat -d src -x "*.tmp"


Version History
---------------
See `CHANGELOG.md <./CHANGELOG.md>`_ for detailed history.

- **0.3.0 (2025-04-24):** Major refactor. Replaced ignore handling with `gocodewalker` for recursive Git-compatible behavior. Added `-n/--no-scan`. Split code into multiple files under `cmd/codecat/`. Fixed bugs related to excludes, non-existent dirs, and gitignore logic. Reverted to `--no-gitignore` flag.
- **0.2.2 (2025-04-23):** Bugfix release (internal).
- **0.2.1 (2025-04-20):** Project Renamed to `codecat`.
- **0.1.0 (2025-04-19):** Initial version (`food4ai`).


To-Do and Known Problems
------------------------
See `TODO.rst <./TODO.rst>`_.

---