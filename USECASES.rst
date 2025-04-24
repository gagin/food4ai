.. _codecat-usecases:

CodeCat Use Cases
=================

:Project: CodeCat
:Description: A utility to concatenate the contents of development files into a single file for use in an LLM's context window. Supports optional ``.gitignore`` rules, configurable exclusion/inclusion lists, default file extensions, forced inclusions, explicit directory scanning, and a summary printout with a pseudo-graphic tree.
:Date: April 23, 2025
:Author: xAI (via Grok 3)

Use Case 1: Printout Code from Current Directory with Default Settings, Capture with Shell Redirect
------------------------------------------------------------------------------------------------

:Use Case ID: UC-001
:Actors: Developer, CodeCat
:Description: A developer runs CodeCat in the current directory with default settings to concatenate files for LLM context, capturing the output via shell redirect. Utility info (e.g., summary) is printed to stderr to avoid interfering with the redirected output.
:Preconditions:
  - Current directory contains development files (e.g., ``.py``, ``.js``).
  - Config file (``~/.config/codecat/config.toml``) exists with default extensions (e.g., ``.py``, ``.js``, ``.ts``, ``.java``).
:Postconditions: A single file containing concatenated code is created via redirect.
:Priority: High
:Frequency: Daily
:Status: Draft

Basic Flow
~~~~~~~~~~

1. Developer navigates to the project directory.
2. Developer runs ``codecat -e jsonc > output.txt``.
3. CodeCat reads default config (extensions: ``.py``, ``.js``, ``.ts``, ``.java``).
4. CodeCat scans the current directory (``.``), including files matching the extensions and ``.jsonc``.
5. CodeCat outputs concatenated file contents to stdout, redirected to ``output.txt``.
6. CodeCat prints a summary to stderr: stderr, including:

   - Pseudo-graphic tree of included files with sizes.
   - List of skipped and empty files.
   - Total size of the output file.

Core Scenario
~~~~~~~~~~~~~

The developer needs to quickly gather all relevant code files in the current directory to paste into an LLM’s context for analysis or debugging.

Feature-Specific Scenario
~~~~~~~~~~~~~~~~~~~~~~~~

Uses CodeCat’s default mode (no flags except ``-e jsonc``), scanning the current directory and relying on config-defined extensions. Utility info goes to stderr to keep stdout clean for redirection.

Needs
~~~~~

- Quick, minimal-command way to concatenate code for LLM use.
- Support for project-specific extensions (e.g., JSONC).
- Avoid cluttering redirected output with utility info.

Choices/Compromises/Assumptions
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

- **Choice**: Default mode assumes current directory and config extensions for simplicity.
- **Compromise**: Requires shell redirect (``>``) instead of a built-in output file flag to keep the command minimal.
- **Assumption**: Config file exists with sensible defaults; stderr is appropriate for summary.

Solution
~~~~~~~~

**Command**: ``codecat -e jsonc > output.txt``

**Example Summary (stderr)**::

    📁 Project Root
    ├── src/main.py (2 KB)
    ├── src/utils.js (1 KB)
    ├── config.jsonc (0.5 KB)
    └── total: 3.5 KB
    Skipped: node_modules/, .env
    Empty: none

**Output**: ``output.txt`` contains concatenated contents of ``main.py``, ``utils.js``, and ``config.jsonc``.

Use Case 2: Use Gitignore for Sensitive and Temp Files, Add Sample Doc Directories to Exclusion List in Config
-----------------------------------------------------------------------------------------------------------

:Use Case ID: UC-002
:Actors: Developer, CodeCat
:Description: A developer configures CodeCat to respect ``.gitignore`` rules and exclude sample/doc directories via the config file, then runs it to concatenate files from the current directory for LLM context.
:Preconditions:
  - Current directory contains development files and a ``.gitignore`` file.
  - Config file (``~/.config/codecat/config.toml``) includes exclusions for ``docs/`` and ``samples/``.
:Postconditions: A single file containing concatenated code (excluding ignored and configured directories) is output.
:Priority: Medium
:Frequency: Weekly
:Status: Draft

Basic Flow
~~~~~~~~~~

1. Developer edits ``~/.config/codecat/config.toml`` to exclude ``docs/`` and ``samples/``:

   .. code-block:: toml

      extensions = [".py", ".js", ".ts", ".java"]
      exclude = ["docs/", "samples/"]

2. Developer navigates to the project directory.
3. Developer runs ``codecat -d . --gitignore > output.txt``.
4. CodeCat reads config and ``.gitignore`` (e.g., ignoring ``node_modules/``, ``.env``).
5. CodeCat scans the specified directory (``.``), including files matching extensions and excluding ``docs/``, ``samples/``, and ``.gitignore`` patterns.
6. CodeCat outputs concatenated file contents to stdout, redirected to ``output.txt``.
7. CodeCat prints a summary to stderr.

Core Scenario
~~~~~~~~~~~~~

The developer wants to concatenate code files but exclude sensitive/temporary files (via ``.gitignore``) and irrelevant documentation/sample directories.

Feature-Specific Scenario
~~~~~~~~~~~~~~~~~~~~~~~~

Leverages ``.gitignore`` support and config-based exclusions to filter out unwanted files. Explicit ``-d .`` is required since flags are used.

Needs
~~~~~

- Avoid including sensitive files (e.g., ``.env``) or temporary files (e.g., ``node_modules/``).
- Exclude project-specific documentation/sample directories irrelevant to the LLM.

Choices/Compromises/Assumptions
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

- **Choice**: ``.gitignore`` support reuses existing project conventions.
- **Compromise**: Requires explicit ``-d`` when using flags, adding slight complexity.
- **Assumption**: ``.gitignore`` and config file are correctly formatted.

Solution
~~~~~~~~

**Command**: ``codecat -d . --gitignore > output.txt``

**Example Config**: ``~/.config/codecat/config.toml`` (see above).

**Example Summary (stderr)**::

    📁 Project Root
    ├── src/main.py (2 KB)
    ├── src/utils.js (1 KB)
    └── total: 3 KB
    Skipped: node_modules/, .env, docs/, samples/
    Empty: none

**Output**: ``output.txt`` contains concatenated contents of ``main.py`` and ``utils.js``.

Use Case 3: Exclude README for Current Printout, Model Doesn't Need It
---------------------------------------------------------------------

:Use Case ID: UC-003
:Actors: Developer, CodeCat
:Description: A developer runs CodeCat to concatenate files but excludes ``README.md`` via command-line argument to keep the LLM context focused on code.
:Preconditions:
  - Current directory contains development files and a ``README.md``.
  - Config file (``~/.config/codecat/config.toml``) exists with default extensions.
:Postconditions: A single file containing concatenated code (excluding ``README.md``) is output.
:Priority: Medium
:Frequency: Occasional
:Status: Draft

Basic Flow
~~~~~~~~~~

1. Developer navigates to the project directory.
2. Developer runs ``codecat -d . -x README.md > output.txt``.
3. CodeCat reads default config (extensions: ``.py``, ``.js``, ``.ts``, ``.java``).
4. CodeCat scans the specified directory (``.``), including files matching extensions but excluding ``README.md``.
5. CodeCat outputs concatenated file contents to stdout, redirected to ``output.txt``.
6. CodeCat prints a summary to stderr.

Core Scenario
~~~~~~~~~~~~~

The developer needs to provide code to an LLM for analysis but exclude documentation like ``README.md``.

Feature-Specific Scenario
~~~~~~~~~~~~~~~~~~~~~~~~

Uses ad-hoc exclusion via ``-x``/``--exclude`` to skip ``README.md`` for this specific run.

Needs
~~~~~

- Keep LLM context focused on code, not project documentation.
- Flexible, one-off exclusion without modifying config.

Choices/Compromises/Assumptions
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

- **Choice**: ``-x`` allows quick exclusions without config changes.
- **Compromise**: Requires explicit ``-d`` when using flags.
- **Assumption**: ``README.md`` is the primary documentation file to exclude.

Solution
~~~~~~~~

**Command**: ``codecat -d . -x README.md > output.txt``

**Example Summary (stderr)**::

    📁 Project Root
    ├── src/main.py (2 KB)
    ├── src/utils.js (1 KB)
    └── total: 3 KB
    Skipped: README.md
    Empty: none

**Output**: ``output.txt`` contains concatenated contents of ``main.py`` and ``utils.js``.

Use Case 4: Exclude Tests Directory to Ask LLM to Create Fresh Unit Tests
------------------------------------------------------------------------

:Use Case ID: UC-004
:Actors: Developer, CodeCat
:Description: A developer runs CodeCat to concatenate files but excludes the ``tests/`` directory to provide code without existing tests, enabling the LLM to generate fresh unit tests.
:Preconditions:
  - Current directory contains development files and a ``tests/`` directory.
  - Config file (``~/.config/codecat/config.toml``) exists with default extensions.
:Postconditions: A single file containing concatenated code (excluding ``tests/``) is output.
:Priority: Medium
:Frequency: Occasional
:Status: Draft

Basic Flow
~~~~~~~~~~

1. Developer navigates to the project directory.
2. Developer runs ``codecat -d . -x tests/ > output.txt``.
3. CodeCat reads default config (extensions: ``.py``, ``.js``, ``.ts``, ``.java``).
4. CodeCat scans the specified directory (``.``), including files matching extensions but excluding the ``tests/`` directory.
5. CodeCat outputs concatenated file contents to stdout, redirected to ``output.txt``.
6. CodeCat prints a summary to stderr.

Core Scenario
~~~~~~~~~~~~~

The developer wants to provide code to an LLM to generate new unit tests, excluding existing tests.

Feature-Specific Scenario
~~~~~~~~~~~~~~~~~~~~~~~~

Uses directory exclusion with a trailing slash (``-x tests/``) to skip the entire ``tests/`` directory.

Needs
~~~~~

- Exclude existing tests to avoid influencing the LLM’s test generation.
- Support directory-level exclusions for flexibility.

Choices/Compromises/Assumptions
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

- **Choice**: Trailing slash (``tests/``) clearly indicates directory exclusion.
- **Compromise**: Requires explicit ``-d`` when using flags.
- **Assumption**: ``tests/`` is the standard directory for test files.

Solution
~~~~~~~~

**Command**: ``codecat -d . -x tests/ > output.txt``

**Example Summary (stderr)**::

    📁 Project Root
    ├── src/main.py (2 KB)
    ├── src/utils.js (1 KB)
    └── total: 3 KB
    Skipped: tests/
    Empty: none

**Output**: ``output.txt`` contains concatenated contents of ``main.py`` and ``utils.js``.

Use Case 5: Use -o/--output to Specify Output File and Print Utility Info to Stdout
---------------------------------------------------------------------------------

:Use Case ID: UC-005
:Actors: Developer, CodeCat
:Description: A developer runs CodeCat with an explicit output file (``-o``) to concatenate files, with utility info (e.g., summary) printed to stdout for visibility in the terminal.
:Preconditions:
  - Current directory contains development files.
  - Config file (``~/.config/codecat/config.toml``) exists with default extensions.
:Postconditions: A single file containing concatenated code is written to the specified output file.
:Priority: Medium
:Frequency: Weekly
:Status: Draft

Basic Flow
~~~~~~~~~~

1. Developer navigates to the project directory.
2. Developer runs ``codecat -d . -o output.txt``.
3. CodeCat reads default config (extensions: ``.py``, ``.js``, ``.ts``, ``.java``).
4. CodeCat scans the specified directory (``.``), including files matching extensions.
5. CodeCat writes concatenated file contents to ``output.txt``.
6. CodeCat prints a summary to stdout.

Core Scenario
~~~~~~~~~~~~~

The developer wants to save concatenated code to a file and see the summary in the terminal.

Feature-Specific Scenario
~~~~~~~~~~~~~~~~~~~~~~~~

Uses ``-o``/``--output`` to specify the output file, with utility info redirected to stdout instead of stderr.

Needs
~~~~~

- Save output to a file without manual redirection.
- View the summary directly in the terminal during the run.

Choices/Compromises/Assumptions
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

- **Choice**: ``-o`` simplifies output file specification; stdout for summary aligns with explicit output mode.
- **Compromise**: Requires explicit ``-d`` when using flags.
- **Assumption**: Developers expect stdout for run info when ``-o`` is used.

Solution
~~~~~~~~

**Command**: ``codecat -d . -o output.txt``

**Example Summary (stdout)**::

    📁 Project Root
    ├── src/main.py (2 KB)
    ├── src/utils.js (1 KB)
    └── total: 3 KB
    Skipped: none
    Empty: none

**Output**: ``output.txt`` contains concatenated contents of ``main.py`` and ``utils.js``.

Use Case 6: Pipe Output to LLM Command (Future Feature)
------------------------------------------------------

:Use Case ID: UC-006
:Actors: Developer, CodeCat, LLM Command
:Description: A developer runs CodeCat to concatenate files and pipes the output directly to an ``llm`` command for seamless LLM integration, avoiding intermediate files.
:Preconditions:
  - Current directory contains development files.
  - Config file (``~/.config/codecat/config.toml``) exists with default extensions.
  - ``llm`` command is installed and configured.
:Postconditions: Concatenated code is processed by the ``llm`` command.
:Priority: Low (Future Feature)
:Frequency: TBD
:Status: Draft

Basic Flow
~~~~~~~~~~

1. Developer navigates to the project directory.
2. Developer runs ``codecat | llm``.
3. CodeCat reads default config (extensions: ``.py``, ``.js``, ``.ts``, ``.java``).
4. CodeCat scans the current directory (``.``), including files matching extensions.
5. CodeCat outputs concatenated file contents to stdout, piped to ``llm``.
6. CodeCat prints a summary to stderr.
7. The ``llm`` command processes the input and produces its output.

Core Scenario
~~~~~~~~~~~~~

The developer wants to streamline the workflow by piping CodeCat’s output directly to an LLM command.

Feature-Specific Scenario
~~~~~~~~~~~~~~~~~~~~~~~~

Uses piping to integrate CodeCat with an ``llm`` command, keeping utility info on stderr.

Needs
~~~~~

- Eliminate intermediate files for faster LLM workflows.
- Maintain compatibility with default mode (stderr for summary).

Choices/Compromises/Assumptions
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

- **Choice**: Piping leverages stdout, consistent with default mode.
- **Compromise**: Requires ``llm`` command to handle piped input correctly.
- **Assumption**: ``llm`` command is a standard tool in the developer’s workflow.

Solution
~~~~~~~~

**Command**: ``codecat | llm``

**Example Summary (stderr)**::

    📁 Project Root
    ├── src/main.py (2 KB)
    ├── src/utils.js (1 KB)
    └── total: 3 KB
    Skipped: none
    Empty: none

**Output**: Concatenated contents of ``main.py`` and ``utils.js`` are piped to ``llm``.

Notes
=====

- **Config File**: The ``config.toml`` format is assumed for consistency. Developers can override it with ``--config``.
- **Extensions**: Default extensions are ``.py``, ``.js``, ``.ts``, ``.java`` for illustration. Projects may customize these.
- **Future Features**: Use Case 6 (piping to ``llm``) is marked as a future feature and may require additional implementation.
- **Forced Inclusion**: Not explicitly covered but can be addressed with ``-f`` (e.g., ``codecat -d . -f specific.py``) in future use cases.

For feedback or additional use cases, contact the CodeCat team.