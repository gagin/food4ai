food4ai
=======

A command-line tool to concatenate source code files into a single output,
formatted for easy consumption by Large Language Models (LLMs) or other AI
analysis tools.

Purpose
-------

When working with AI models for code analysis, refactoring, or question
answering, it's often necessary to provide the model with context from your
codebase. Manually copying and pasting files is tedious and error-prone.

``food4ai`` automates this process by scanning a target directory, filtering
files based on extensions and exclusion patterns (including ``.gitignore``),
and concatenating their contents into a single standard output stream. Each
file's content is clearly delimited with markers indicating the filename.

Alternative Approach: Using LLMs Directly
------------------------------------------

As an alternative to using this utility, you could achieve a similar result by:

1.  Generating a file listing of your project, for example using the ``tree -s`` command (which includes file sizes).
2.  Providing this file listing to a capable Large Language Model (LLM).
3.  Asking the LLM to generate a shell script (e.g., bash or PowerShell) that concatenates the content of all the necessary files into a single output file.

This approach might offer more flexibility in selecting specific files or tailoring the output format directly via the LLM prompt. However, ``food4ai`` provides a dedicated, configurable, and potentially faster command-line solution for this common task.

Installation
------------

Assuming you have Go installed, you can build the tool from source:

.. code-block:: bash

    git clone https://github.com/gagin/food4ai.git
    cd food4ai
    go build -o food4ai .

This will create an executable named ``food4ai`` in the current directory.
You may want to move this executable to a directory in your system's PATH
(e.g., ``/usr/local/bin`` or ``~/bin``) to run it from anywhere.

Usage
-----

The tool is run from the command line. It accepts several flags to control
its behavior.

.. code-block:: bash

    food4ai [flags] [target_directory]

**Arguments:**

*   ``target_directory`` (optional): The target directory to scan recursively. Defaults to the current directory (``.``). Can also be specified using the ``-d`` flag.

**Flags:**

*   **-d, --directory** *path*
    The target directory to scan recursively. Overrides the positional ``target_directory`` argument if both are provided. Defaults to the current directory (``.``).

*   **-e, --extensions** *ext1,ext2,...*
    Comma-separated list of file extensions (without leading dot) to include.
    Can be repeated. Overrides extensions specified in the config file.
    Defaults to the extensions listed in the config file or a hardcoded list if no config is found.

*   **-f, --files** *path1,path2,...*
    Comma-separated list of specific file paths to include manually. Can be
    repeated. These files bypass extension filters, exclusion patterns, and
    ``.gitignore`` rules. Paths can be absolute or relative.

*   **-x, --exclude** *pattern1,pattern2,...*
    Comma-separated list of glob patterns to exclude. Can be repeated. These
    patterns are matched against paths relative to the target directory.
    Overrides exclusion patterns specified in the config file. Defaults to
    patterns listed in the config file if present.

*   **--no-gitignore**
    Disable processing of ``.gitignore`` files found in the target directory and its subdirectories.
    By default, ``.gitignore`` files are processed unless disabled by this flag or
    the config file.

*   **--config** *path*
    Path to a custom configuration file. Defaults to ``~/.config/food4ai/config.toml``.

*   **-h, --help**
    Show help message and exit.

*   **-v, --version**
    Show version information and exit.


Configuration
-------------

``food4ai`` can be configured using a file, typically located at
``~/.config/food4ai/config.toml`` (this path can be changed with the ``--config`` flag).
This file uses the TOML format.

An example configuration file (`config.toml`) is provided in the repository.
The available options are:

*   ``header_text`` (string): The introductory text placed at the very
    beginning of the output.
*   ``include_extensions`` (array of strings): List of file extensions
    (without leading dot) to include by default during scans. Command-line
    flags (``-e``) override this.
*   ``exclude_patterns`` (array of strings): List of glob patterns to exclude
    by default during scans. These apply relative to the target directory.
    Manually added files (``-f``) are NOT affected by these. Command-line
    flags (``-x``) override this.
*   ``comment_marker`` (string): The marker used to delimit file sections in
    the output. Make sure it's unlikely to appear naturally at the start/end
    of lines in your code. Defaults to ``---``.
*   ``use_gitignore`` (boolean): Whether to respect ``.gitignore`` files found
    in the target directory by default. Set to ``false`` to disable. Can be
    overridden by the ``--no-gitignore`` command-line flag.

Command-line flags take precedence over the configuration file settings.

Output Format
-------------

The output is printed to standard output (stdout). It begins with the
configured ``header_text`` (if any), followed by the content of each included file.
Each file's content is preceded by a line containing the ``comment_marker``
followed by the file path (relative to the target directory), and followed by
a line containing just the ``comment_marker``.

Example:

.. code-block:: text

    Codebase for analysis:

    --- path/to/your/file.py
    print("Hello, world!")
    ---

    --- another/file.js
    console.log("Another file");
    ---

Empty files are not included with markers, but a list of empty files found
is appended at the end of the output, sent to standard error (stderr).

Logging
-------

Informational messages (like files being processed or skipped), warnings, errors,
and the list of empty files are printed to standard error (stderr). This allows
you to redirect the code output (stdout) to a file or pipe it to another
command without mixing it with log messages.

Example Usage
-------------

Concatenate all ``.go`` and ``.mod`` files in the current directory and its
subdirectories, excluding files in any ``vendor`` directory:

.. code-block:: bash

    food4ai -e go,mod -x "vendor/*" .

Concatenate a specific Python file (`src/main.py`) and all ``.js`` files in the `frontend`
directory, disabling ``.gitignore`` processing, and save to a file:

.. code-block:: bash

    food4ai -f src/main.py -d frontend -e js --no-gitignore > project_context.txt
