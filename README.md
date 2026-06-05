# Einstar Vega CLI

A command-line tool and Terminal UI (TUI) for the Einstar Vega 3D Scanner. It allows you to explore the device's file system, view hardware details, and quickly download projects or individual files over the network without having to make an account or register your device (in other words, **fuck you** Einstar).

Note that the Vega uses a proprietary project format, meaning raw files (like `mesh.beb`) cannot be directly imported into standard 3D software yet—but freeing the data from the device is the first step. Hopefully, the community will eventually be able to decode `mesh.beb` on the fly.

Tested on firmware `v1.3.0-22`

## Installation

Download the latest pre-compiled binary for macOS, Linux, or Windows from the [Releases](https://github.com/alexgorbatchev/einstar-vega-cli/releases) page.

1. Download the archive for your operating system (e.g., `vega-cli_Windows_x86_64.zip` or `vega-cli_Darwin_arm64.tar.gz`).
2. Extract the archive.
3. Run the `vega-cli` executable from your terminal.

*(If you prefer to download via the command line, you can use the GitHub CLI: `gh release download --repo alexgorbatchev/einstar-vega-cli`)*

## Usage

Just connect your camera via cable and run the binary - it will open an interactive UI that shows the available projects.

```sh
./vega-cli
```

If your scanner has a different IP address or you want to output files to a different directory:

```sh
./vega-cli -a 192.168.1.100 -o /my/custom/output
```

**TUI Controls:**
*   `Arrow Keys` - Navigate the file tree
*   `Enter` - Expand/collapse directories
*   `Space` - Multi-select files and directories
*   `d` - Download selected (or highlighted) files/directories
*   `q` - Quit the application
*   `x` / `r` - Delete / Rename (Experimental - may be unsupported by the device API)

## Acknowledgements

This Go implementation was inspired by the original Python extraction script: [Tigra-10/einstar-vega-extract](https://github.com/Tigra-10/einstar-vega-extract)

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
