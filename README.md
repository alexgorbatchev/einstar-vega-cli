# Einstar Vega CLI

Relatively simple Go application to interact with your Einstar Vega 3D Scanner. It uses an interactive Terminal UI (TUI) to let you explore the file system of the scanner, check device details, and download projects or files efficiently.

Could help a bit if you are using Linux. A bit - because the project format of the Vega is proprietary, so you will not be able to find there something that could be used immediately (like mesh ply files). Yeah that's not good - what's the point of having a dedicated 3D Scanner device, which is useless without PC, right? But maybe someday we will be able to decode `mesh.bib` on the fly as well.

Tested on `v1.3.0-22`

## Installation

You need [Go](https://go.dev/) to build the binary.

```sh
go build -o vega-cli ./cmd/vega-cli
```

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
