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

Installation
------------

Assuming you have Go installed, you can build the tool from source:

.. code-block:: bash

    git clone <repository_url> # Replace with the actual repository URL
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

    food4ai [flags]

Flags:

.. program:: food4ai

.. option:: -d, --directory <path>

   The target directory to scan recursively. Defaults to the current directory (``.``).

.. option:: -e, --extensions <ext1,ext2,...>

   Comma-separated list of file extensions (without leading dot) to include.
   Can be repeated. Overrides extensions specified in the config file.
   Defaults to the extensions listed in the config file or a hardcoded list.

.. option:: -f, --files <path1,path2,...>

   Comma-separated list of specific file paths to include manually. Can be
   repeated. These files bypass extension filters, exclusion patterns, and
   ``.gitignore`` rules. Paths can be absolute or relative.

.. option:: -x, --exclude <pattern1,pattern2,...>

   Comma-separated list of glob patterns to exclude. Can be repeated. These
   patterns are matched against paths relative to the target directory.
   Overrides exclusion patterns specified in the config file. Defaults to
   patterns listed in the config file.

.. option:: --no-gitignore

   Disable processing of ``.gitignore`` files found in the target directory.
   By default, ``.gitignore`` is processed unless disabled by this flag or
   the config file.

Configuration
-------------

``food4ai`` can be configured using a file located at
``~/.config/food4ai/config.toml``. This file uses the TOML format.

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
    of lines in your code.
*   ``use_gitignore`` (boolean): Whether to respect ``.gitignore`` files found
    in the target directory by default. Set to ``false`` to disable. Can be
    overridden by the ``--no-gitignore`` command-line flag.

Command-line flags take precedence over the configuration file.

Output Format
-------------

The output is printed to standard output (stdout). It begins with the
configured ``header_text``, followed by the content of each included file.
Each file's content is preceded by a line containing the ``comment_marker``
followed by the file path (relative to the current working directory if
possible), and followed by a line containing just the ``comment_marker``.

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
is appended at the end of the output.

Logging
-------

Informational messages, warnings, and errors are printed to standard error
(stderr). This allows you to redirect the code output (stdout) to a file or
pipe it to another command without mixing it with log messages.

Example Usage
-------------

Concatenate all ``.go`` and ``.mod`` files in the current directory and its
subdirectories, excluding files in a ``vendor`` directory:

.. code-block:: bash

    food4ai -e go,mod -x "vendor/*"

Concatenate a specific Python file and all ``.js`` files in the ``frontend``
directory, disabling ``.gitignore``:

.. code-block:: bash

    food4ai -f src/main.py -d frontend -e js --no-gitignore > frontend_code.txt
