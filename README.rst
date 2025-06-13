==========
README.rst
==========

codecat
=======
**Version:** 0.4.2

A command-line tool to concatenate source code files into a single output,
formatted for easy consumption by Large Language Models (LLMs) or other AI
analysis tools. Uses `gocodewalker <https://github.com/boyter/gocodewalker>`_ for robust, Git-compatible
``.gitignore`` / ``.ignore`` handling.

Purpose
-------

When working with AI models for code analysis, refactoring, or question
answering, it's often necessary to provide the model with context from your
codebase. Manually copying and pasting files is tedious and error-prone.

``codecat`` automates this process by scanning target directories, filtering
files based on extensions and multiple exclusion mechanisms (global config,
project config, command-line, gitignore), and concatenating their contents
into a single output stream or file. Each file's content is clearly delimited
with markers indicating the filename relative to your **Current Working Directory (CWD)**.
A summary of included files, sizes, and any errors is printed separately.

See `USECASES.jsonc <./USECASES.jsonc>`_ for detailed usage scenarios.
See `TODO.rst <./TODO.rst>`_ for planned features and improvements.


Installation
------------
Assuming you have Go installed and your GOPATH/GOBIN is set up:

**Option 1: Build from Source (Recommended for Dev)**

.. code-block:: bash

    git clone https://github.com/gagin/codecat.git
    cd codecat
    # Build and install to ~/go/bin/ or $GOPATH/bin
    go install ./cmd/codecat
    # Or build locally
    go build -o codecat ./cmd/codecat

**Option 2: Use Makefile (Convenient for local install)**

.. code-block:: bash

    git clone https://github.com/gagin/codecat.git
    cd codecat
    # Installs to ~/.local/bin (ensure this is in your PATH)
    make local-bin

Ensure the resulting ``codecat`` executable is in a directory included in your
system's ``PATH`` environment variable (e.g., ``/usr/local/bin``,
``~/.local/bin``, ``~/go/bin``) to run it from anywhere.


Usage
-----

The tool operates relative to your **Current Working Directory (CWD)**. File paths
in the output and exclusion patterns (except ``exclude_basenames`` and ``.gitignore``)
are interpreted relative to the CWD.

**Modes:**

1.  **Positional Argument:** ``codecat [target_directory] [flags]``
    Scan *only* the specified ``target_directory`` (path relative to CWD or absolute). Cannot be used with ``-d``.
    If ``target_directory`` is omitted, defaults to scanning the CWD (``.``).

2.  **Flags Only:** ``codecat [flags]``
    Use flags for specific control. No positional arguments allowed.
    *   If ``-d`` is used, scan the specified directories.
    *   If ``-d`` is omitted and ``-n`` (no-scan) is NOT used, scan the CWD (``.``).
    *   If ``-n`` is used, ``-d`` is ignored, and directory scanning is skipped.

**Flags:**

*   **-d, --directory** *path1[,path2,...]*
    Comma-separated list of target directories/paths to scan (relative to CWD or absolute). Use this *or* a positional argument. Ignored if ``-n`` is used. Defaults to scanning CWD if no positional argument or ``-n`` is provided.

*   **-e, --extensions** *ext1,ext2,...*
    Comma-separated list of file extensions (without leading dot, e.g., ``py,go,js``) to include. Can be repeated. Overrides config's ``include_extensions``.

*   **-f, --files** *path1,path2,...*
    Comma-separated list of specific file paths (relative to CWD or absolute) to include manually. **Highest priority:** Bypasses directory-based exclusions (like ``-x test_data``) and ``.gitignore``. This is the **only** way to include specific extensionless files (like ``Makefile`` or ``LICENSE``).

*   **-x, --exclude** *pattern1,pattern2,...*
    Comma-separated list of paths related to exclude. Matched against paths relative to **CWD**. Doesn't supports globs/wildcards or partial names. Adds to patterns from ``.codecat_exclude``.

    *   ``path/to/file.txt``: Excludes that specific file.
    *   ``build``: Excludes a file or directory named ``build`` relative to CWD *and* all contents if it's a directory (trailing slash **not** required). Directory ``deeper/build`` will still be included.

*   **--no-gitignore**
    Disable processing of ``.gitignore`` and ``.ignore`` files found recursively. By default (without this flag), Git-compatible recursive ignore processing is enabled. Overrides config's ``use_gitignore`.

*   **-n, --no-scan**
    Skip directory scanning entirely. Only processes files specified manually via ``-f``. Requires ``-f`` to produce output.

*   **-o, --output** *path*
    Write concatenated code to *path* instead of stdout. Summary/logs go to stdout. If omitted, code goes to stdout and summary/logs go to stderr.

*   **--config** *path*
    Path to a custom configuration file. Defaults to ``~/.config/codecat/config.toml``.

*   **--loglevel** *(debug|info|warn|error)*
    Set logging verbosity. Defaults to ``warn``. Logs go to stderr (or stdout if ``-o`` is used).

*   **-h, --help**
    Show help message and exit.

*   **-v, --version**
    Show version information and exit.


Configuration & Exclusions
--------------------------
``codecat`` uses a hierarchy of exclusion rules and settings, loaded from
``~/.config/codecat/config.toml`` (or ``--config`` path) and project files.

**Recommendation:** Copy ``config.toml.example`` to ``~/.config/codecat/config.toml``
and customize it with your preferred default extensions and global basename exclusions.

**1. Global Config (`config.toml`)**

Located at ``~/.config/codecat/config.toml`` by default.

*   **`exclude_basenames = [...]`**:

    *   There's a **BUG** currently where only full directory names in parent chain are excluded with this rule, no substrings of file name matching.
    *   A list of **glob patterns** matched against the **basename** (the final file or directory name) of any item encountered during scanning *or* listed via ``-f``.
    *   **Use Case:** Globally excluding common names like ``node_modules``, ``*.log``, ``build``, ``.DS_Store``, etc., regardless of where they appear in *any* project you run ``codecat`` on. Offers broader, name-based exclusion than typical path-relative ``.gitignore``.
    *   These patterns are checked *first*. <strikethrough>If a directory basename matches, the directory and its contents are excluded (unless a file within is specified with ``-f``).
    *   Defaults include common VCS, build, cache, log, and OS metadata files/dirs.

*   **`include_extensions = [...]`**:

    *   Default list of extensions (e.g., "py", "go", "js") to include during scans.
    *   Overridden by the ``-e`` flag if used.
    *   **Note:** Files without extensions (like ``Makefile``, ``LICENSE``) are **not** included by default during scans. Use the ``-f`` flag to include specific extensionless files.

*   **`use_gitignore = true | false`**:

    *   Whether to enable recursive ``.gitignore`` / ``.ignore`` processing by default.
    *   Overridden by ``--no-gitignore``.

*   **`header_text = "..."`**:

    *   Optional text prepended to the output. Include trailing ``\n`` within the string if desired, as no extra newlines are added automatically after the header. Default includes one ``\n``.

*   **`comment_marker = "---"`**:

    *   The string used to delimit file sections.

**2. Project Config (`.codecat_exclude`)**

*   If a file named ``.codecat_exclude`` exists in the **Current Working Directory (CWD)** where you run ``codecat``, it is loaded.
*   Each line is treated as a **CWD-relative glob pattern**, identical in syntax and behavior to patterns provided via the ``-x`` flag.
*   **Use Case:** Project-specific exclusions that shouldn't be global (e.g., ``data/``, ``notebooks/archive``, ``internal/legacy_code``) or exclusions you don't want in ``.gitignore``.
*   Lines starting with ``#`` are ignored as comments.
*   See ``.codecat_exclude.example``.

**3. Command Line Flags (`-x`, `--no-gitignore`, `-f`)**

*   ``-x`` patterns are added to patterns from ``.codecat_exclude``. They are CWD-relative globs.
*   ``--no-gitignore`` overrides ``use_gitignore = true``.
*   ``-f`` provides the highest inclusion priority (see Flags section).

**Exclusion Precedence:**

When deciding whether to **exclude** an item found during a **scan**:

1.  Is it inside a directory already marked for exclusion by a previous basename or CWD-relative pattern match on the parent directory? (If yes, exclude).
2.  Does its **basename** match any pattern in ``exclude_basenames``? (If yes, exclude; mark dir if applicable).
3.  Does its **CWD-relative path** match any pattern from ``.codecat_exclude`` or ``-x`` (using both exact/glob and directory prefix logic)? (If yes, exclude; mark dir if applicable).
4.  If ``use_gitignore`` is enabled, does it match a relevant ``.gitignore`` / ``.ignore`` rule? (If yes, exclude).

When deciding whether to **exclude** a file specified via **-f**:

1.  Does its **basename** match any pattern in ``exclude_basenames``? (If yes, exclude).
2.  Does its **CWD-relative path** match any *non-directory* pattern from ``.codecat_exclude`` or ``-x``? (If yes, exclude). (It ignores directory patterns like `-x mydir`).

**Excluding Directories without Trailing Slash:**

You **do not** need a trailing slash for patterns in ``-x`` or ``.codecat_exclude`` to exclude a directory and its contents during scanning.
*   ``-x build`` will exclude a file named `build` *or* a directory named `build` (and its contents).
*   ``-x path/to/dir`` will exclude the directory `path/to/dir` and its contents.

**Advanced Exclusions using Shell:**

For complex patterns not supported by standard globs (like recursive directory searches), you can use shell commands like ``find`` to generate a comma-separated list for ``-x``.

*Example: Exclude all `*.test.js` files anywhere under `src/`*

.. code-block:: bash

    # Use find to locate files and print paths, then join with commas
    # Note: Assumes filenames don't contain commas or newlines
    EXCLUDES=$(find src -name '*.test.js' -print | paste -sd,)
    codecat -x "$EXCLUDES" ...

*Example: Exclude all directories named `__tests__`*

.. code-block:: bash

    # Use find to locate directories and print paths, then join with commas
    EXCLUDES=$(find . -type d -name '__tests__' -print | paste -sd,)
    codecat -x "$EXCLUDES" ...


Output Format
-------------

**Concatenated Code:**
* Sent to stdout by default, or to the file specified by ``-o``.
* Starts with ``header_text`` from config (if any, printed exactly as defined).
* Each included file's content is wrapped by marker lines indicating the path relative to the **CWD**:
    .. code-block:: text

        Codebase for analysis:
        --- src/main.go
        package main
        //...
        ---
        --- internal/helper.go
        package internal
        // ...
        ---

**Summary & Logs:**
* Sent to stderr by default, or to stdout if ``-o`` is used.
* Includes messages based on ``--loglevel`` (default ``warn``).
* Ends with a summary section detailing the operation results:
    .. code-block:: text

        --- Summary ---
        Included 2 files (1.5 KiB total) relative to CWD '/path/to/project':
        ├── src
        │   └── main.go (1.1 KiB) [M]
        └── internal
            └── helper.go (450 B)

        Empty files found (1):
        - config/empty.yaml

        Errors encountered (1):
        - data/unreadable.bin: permission denied
        ---------------

* Manually included files are marked with `[M]` in the tree.


Example Usage
-------------

Scan current directory using defaults (respects ``.gitignore`` recursively, uses config):

.. code-block:: bash

    codecat > output.txt

Scan current directory, disable ``.gitignore``, explicitly exclude ``tests`` dir (relative to CWD), include only ``.go`` files, write to file:

.. code-block:: bash

    codecat --no-gitignore -x tests -e go -o codebase.go.txt

Process only manually specified files (relative to CWD), including ``Makefile``:

.. code-block:: bash

    codecat -n -f Makefile -f cmd/codecat/main.go -f pkg/utils/helpers.go -o core_logic.go.txt

Scan ``src`` dir, use project excludes from ``.codecat_exclude``, use global config, write code to stdout:

.. code-block:: bash

    codecat -d src

Version History
---------------
See `CHANGELOG.rst <./CHANGELOG.rst>`_ for detailed history.

- **0.4.0 (2025-04-25):** Added ``exclude_basenames`` (global), ``.codecat_exclude`` (project), refactored exclusions, simplified CWD-relative dir excludes (no trailing slash needed), changed default log level to ``warn``, header formatting, output newlines. Refactored code structure. Added Makefile and integration tests. Solidified approach for extensionless files (require ``-f``).
- **0.3.0 (2025-04-24):** Major refactor. Replaced ignore handling with ``gocodewalker`` for recursive Git-compatible behavior. Added ``-n/--no-scan``. Split code into multiple files under ``cmd/codecat/``. Fixed bugs related to excludes, non-existent dirs, and gitignore logic. Reverted to ``--no-gitignore`` flag.
- **0.2.x:** Internal refactors, bugfixes, rename to ``codecat``.
- **0.1.0:** Initial version (``food4ai``).


To-Do and Known Problems
------------------------
See `TODO.rst <./TODO.rst>`_.
Biggest ones:

* gitignore is applied from target directory, not project root
* exclude patterns don't work with globs

---
