# PFS - Please Find Solution

`PFS` is a command-line tool that diagnoses and corrects your last failed shell command using a local or hosted Large Language Model (LLM) powered by Ollama.

When you run a command that results in an error (i.e., has a non-zero exit code), simply type `pfs`. The tool will analyze the failed command and provide an explanation of the error and a corrected command. You can then choose to execute the corrected command instantly.

## Features

- **Local First:** Works entirely with your local Ollama models, ensuring privacy and offline capability.
- **Intelligent Correction:** Leverages LLMs to understand the context of your error and provide accurate fixes.
- **Interactive Workflow:** Explains the error and prompts for confirmation before executing any command.
- **Reliable Capture:** A robust shell function ensures that the command, its output, and exit code are all captured correctly for analysis.

## Requirements

- **Go 1.22+**: For building the application.
- **jq**: For safely handling command output in the shell wrapper. You can install it with your system's package manager (e.g., `sudo dnf install jq`, `sudo apt-get install jq`, `brew install jq`).
- **ollama**: The LLM that application talks with. After installing, ollama must serving (`ollama serve &`) and at least 1 model is downloaded to your machine (`ollama pull deepseek-r1:1.5b`).

## Installation

### 1. Clone the Repository

```bash
git clone https://github.com/tatlilimon/PFS.git
cd PFS
```

### 2. Build the Binary

```bash
go build -o pfs ./cmd/
```

### 3. Install the Binary

Move the compiled binary to a directory in your system's `PATH`.

```bash
sudo mv pfs /usr/local/bin/
```

## Configuration

`PFS` uses a `.env` file in your home directory to store your Ollama settings.

### 1. Create the Configuration File

Copy the provided template to `~/.pfs.env`.

```bash
cp .env.template ~/.pfs.env
```

### 2. Edit the Configuration

Open the file and set the `OLLAMA_BASE_URL` for your Ollama instance and the `OLLAMA_MODEL` you wish to use.

```bash
nano ~/.pfs.env
```

### 3. Choosing a Model

The quality of the command correction depends heavily on the model you choose. For best results, use a model that is specifically fine-tuned for code or command-line instructions.

## Shell Setup

To make `PFS` work, you need to source the provided wrapper script in your shell's startup file (e.g., `.bashrc`, `.zshrc`).

### 1. Add the Wrapper to Your Shell Configuration

Open your shell's configuration file:
```bash
# For Bash
nano ~/.bashrc

# For Zsh
nano ~/.zshrc
```

Add the following line to the end of the file. Make sure to use the actual absolute path to the cloned repository.

```bash
source /path/to/your/PFS/pfs_wrapper.sh
```

### 2. Reload Your Shell

For the changes to take effect, either restart your terminal or source your configuration file:

```bash
# For Bash
source ~/.bashrc

# For Zsh
source ~/.zshrc
```

## How to Use

1.  Run any shell command.
2.  If it fails, simply type `pfs` and press Enter.
3.  The tool will provide an explanation and a corrected command.
4.  Press `y` and Enter to execute the new command, or `n` to abort.

## Debugging

`PFS` provides a `--verbose` flag for debugging purposes, which provides detailed output about the interaction with the Ollama model.

Example:
```bash
pfs --verbose
```

## Feel Free to Contribute This Project!
You can help me to developing this app by opening a pull request or issue.