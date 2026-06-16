# Einstar Vega Downloader

A CLI tool and Terminal UI (TUI) to download files from Einstar Vega 3D Scanner **without having to make an account or register your device**. Because frankly, **fuck you** Einstar, I shouldn't have to make an account to use a 3D scanner.

Tested on firmware `v1.3.0-22`

## Installation

Download the latest pre-compiled binary for macOS, Linux, or Windows from the [Releases](https://github.com/alexgorbatchev/einstar-vega-cli/releases) page.

1. Download the archive for your operating system (e.g., `vega-cli_Windows_x86_64.zip` or `vega-cli_Darwin_arm64.tar.gz`).
2. Extract the archive.
3. Plug in the scanner using a high-speed USB-C cable (otherwise transferring large point cloud data will take forever).
4. Run the `vega-cli` executable from your terminal.

## Configuration

You can configure the scanner connection either via command-line flags or environment variables:

| Flag | Environment Variable | Default | Description |
|------|----------------------|---------|-------------|
| `--ip` | `VEGA_IP` | (none) | **Required** - The IP address of the Einstar Vega scanner |
| `--port`, `-p` | `VEGA_PORT` | `8080` | Port used by the Einstar Vega scanner or mock server |
| `--output`, `-o` | `VEGA_OUTPUT` | `projects` | Directory where downloaded files and projects will be stored |

## Usage

### 1. Interactive Terminal UI (TUI)

To launch the full interactive interface to explore and select folders/files to download:

```sh
./vega-cli --ip <scanner_ip>
```

Alternatively, set the environment variable:
```sh
export VEGA_IP=192.168.30.3
./vega-cli
```

**TUI Controls:**
*   `Arrow Keys` - Navigate the file tree
*   `Enter` - Expand/collapse directories
*   `Space` - Multi-select files and directories
*   `d` - Download selected (or highlighted) files/directories
*   `q` - Quit the application
*   `x` / `r` - Delete / Rename (Experimental - may be unsupported by the device API)

### 2. Headless CLI Subcommands

For automation, scripting, or quick inspection, `vega-cli` provides headless subcommands that don't launch the TUI:

#### List Available Projects
List all projects stored on the scanner with their paths and timestamps:
```sh
./vega-cli projects --ip <scanner_ip>
```

#### Get Scanner Hardware Status
Display hardware/firmware details, battery level, storage paths, and active state:
```sh
./vega-cli info --ip <scanner_ip>
```

#### Direct Headless Download
Download an entire project (or all projects) directly from the command line:
```sh
# Download a specific project
./vega-cli download "Project1" --ip <scanner_ip> --output ./my-scans

# Download all available projects
./vega-cli download --all --ip <scanner_ip>
```

### 3. Local Offline Mock Server
Start a mock scanner API server on your local machine to test or inspect the CLI/TUI:
```sh
# Starts mock server on port 8080 (default)
./vega-cli mock

# Start on a custom port
./vega-cli mock --port 9090
```
With the mock server running on a custom port, you can connect the CLI/TUI to it locally:
```sh
./vega-cli --ip localhost --port 9090
```

## Acknowledgements

This Go implementation is a rewrite of the original Python [rabits/einstar-vega-extract](https://github.com/rabits/einstar-vega-extract) script. 

## Details

### Project structure

```
<project name>/
  - cloud.bin  # Pointcloud data?
  - mesh.beb  # Should contain mesh surface data
  - preview.png  # Preview picture
  - scan.proj  # XML file with project parameters
  - frame0/
    - DepthImg_0.dat  # Multiple bin files with num suffix
    - HeadFile.dat
    - RtFile_0.dat  # Multiple bin files with num suffix
    - TexFile_0.dat  # Multiple bin files with num suffix
    - WeightFile_0.dat  # Multiple bin files with num suffix
    - FAST/
      - inu.bin
      - LeftCCF.txt  # Left Far camera params
      - RightCCF.txt  # Right Far camera params
      - TexCCF.txt  # Texture Far camera params
    - HD/
      - LeftCCF.txt  # Left Near camera params
      - RightCCF.txt  # Right Near camera params
      - TexCCF.txt  # Texture Near camera params
```

### Communication

WARNING: It's simple plaintext HTTP on 8080 port with no auth (so please don't connect the device to unknown wifi networks - anyone can get your data!). Usually the device responds with simple binary serialization CBOR, so you can read the response and send POST requests back.

Also it uses websocket (port 8081), but it's unnecessary for our extraction needs.

When you connect the camera via USB-C cable to your computer - it creates a network interface for data transfer.
