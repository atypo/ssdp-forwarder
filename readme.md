# SSDP Forwarder

![Build Status](https://github.com/atypo/ssdp-forwarder/workflows/Build/badge.svg)
![GitHub Release](https://img.shields.io/github/release/atypo/ssdp-forwarder.svg)

**SSDP Forwarder** is a cross-platform application written in Go that listens for SSDP (Simple Service Discovery Protocol) packets on specified network interfaces and ports, then forwards them to designated target ports across other interfaces.

---

## Table of Contents

- [Features](#features)
- [Installation](#installation)
  - [Building from Source](#building-from-source)
  - [Using Pre-Built Binaries](#using-pre-built-binaries)
- [Usage](#usage)
  - [Command-Line Options](#command-line-options)
  - [Examples](#examples)
- [Testing](#testing)

## Features

- **Multi-Interface Support:** Listen on multiple network interfaces simultaneously (-i 'a,b,c').
- **Multi-Port Listening:** Monitor multiple UDP ports for SSDP packets (-i '1990,1900').
- **Optional Target Ports:** Specify different target ports for forwarding SSDP packets (-i '2021').

## Installation

### Building from Source

1. **Clone the Repository:**

   ```sh
   git clone https://github.com/atypo/ssdp-forwarder.git
   cd ssdp-forwarder
   ```

2. **Build the Application:**

   The project includes a `build` script that facilitates cross-compilation for various architectures. The script embeds version information into the binary.

   ```sh
   ./build.sh <GOOS> <GOARCH> <VERSION>
   ```

   **Parameters:**

   - `<GOOS>`: Target Operating System (e.g., `linux`, `windows`, `darwin`).
   - `<GOARCH>`: Target Architecture (e.g., `amd64`, `arm64`, `mips64le`).
   - `<VERSION>`: Version string (e.g., `v1.0.0`).

   **Example:**

   ```sh
   ./build.sh linux amd64 v1.0.0
   ```

   This command will generate the binary in the `build/linux-amd64/` directory as `ssdp-forwarder`.

3. **Run the Application:**

   Navigate to the build directory and execute the binary with the desired flags.

   ```sh
   ./build/linux-amd64/ssdp-forwarder -i "eth0,eth1" -p "1900,1990" -g "239.255.255.250" -d "2021,2022" -v
   ```

### Using Pre-Built Binaries

Pre-built binaries are available in the [Releases](https://github.com/atypo/ssdp-forwarder/releases) section of the GitHub repository. Download the appropriate binary for your platform, extract it, and run as shown in the usage examples below.

## Usage

The SSDP Forwarder is a command-line application that provides various flags to customize its behavior.

### Command-Line Options

- **`-i`**: **(Required)** Comma-separated list of network interface names to listen on.

  ```sh
  -i "eth0,eth1"
  ```

- **`-p`**: **(Required)** Comma-separated list of UDP ports to listen on.

  ```sh
  -p "1900,1990"
  ```

- **`-g`**: **(Required)** Comma-separated list of multicast groups to join.

  ```sh
  -g "239.255.255.250"
  ```

- **`-d`**: **(Optional)** Comma-separated list of target UDP ports to forward to. If omitted, forwarding occurs on the same ports as the listening ports.

  ```sh
  -d "2021,2022"
  ```

- **`-v`**: **(Optional)** Enable verbose/debug logging for detailed monitoring of packet reception and forwarding.

  ```sh
  -v
  ```

- **`--version`**: Display the current version of the SSDP Forwarder.

  ```sh
  --version
  ```

- **`-h` or `--help`**: Display help information.

  ```sh
  -h
  ```

### Examples

#### Basic Usage: Forward on the Same Ports

```sh
sudo ./ssdp-forwarder -i "eth0,eth1" -p "1900,1990" -g "239.255.255.250" -v
```

- **Description:**
  - **Interfaces:** `eth0` and `eth1`
  - **Listening Ports:** `1900` and `1990`
  - **Multicast Group:** `239.255.255.250`
  - **Verbose Logging:** Enabled

#### Advanced Usage: Forward to Different Ports

```sh
sudo ./ssdp-forwarder -i "eth0,eth1" -p "1900,1990" -g "239.255.255.250" -d "2021,2022" -v
```

- **Description:**
  - **Interfaces:** `eth0` and `eth1`
  - **Listening Ports:** `1900` and `1990`
  - **Multicast Group:** `239.255.255.250`
  - **Destination Ports:** `2021` and `2022`
  - **Verbose Logging:** Enabled

## Testing

To verify that the SSDP Forwarder is functioning correctly, perform the following tests:

### 1. Verify Multicast Group Membership

Use `netstat` or `ss` to confirm that your application has joined the specified multicast groups on the designated interfaces and ports.

```sh
netstat -g
```

**Expected Output:**

Look for entries corresponding to your multicast groups and interfaces, such as:

```
IPv4 Local Group Memberships
Interface       Multicast Group
-----------------------------
eth0            239.255.255.250
eth1            239.255.255.250
```

### 2. Sending Test SSDP Packets

Use `socat` or a simple script to send SSDP packets to the multicast group and verify forwarding.

#### Using `socat`:

1. **Send a Test Packet:**

   ```sh
   echo -n "Test SSDP Packet" | socat - UDP4-DATAGRAM:239.255.255.250:1900,interface=eth0
   ```

2. **Listen on Destination Ports:**

   On another interface or machine, listen for incoming packets:

   ```sh
   sudo socat -v UDP4-RECVFROM:2021,ip-add-membership=239.255.255.250:eth1
   sudo socat -v UDP4-RECVFROM:2022,ip-add-membership=239.255.255.250:eth1
   ```

3. **Check Forwarding:**

   The listening instances should receive the "Test SSDP Packet" forwarded to the specified destination ports.

### 3. Monitor Logs

If verbose logging is enabled (`-v` flag), monitor the application logs to verify that packets are received and forwarded correctly.

```sh
./ssdp-forwarder -i "eth0,eth1" -p "1900,1990" -g "239.255.255.250" -d "2021,2022" -v
```

**Sample Log Output:**

```
2025/01/28 17:53:07 Joined group=239.255.255.250 on interface=eth0:1900, localIP=10.0.0.1 (listening & sending to port 2021)
2025/01/28 17:53:07 Joined group=239.255.255.250 on interface=eth0:1990, localIP=10.0.0.1 (listening & sending to port 2022)
2025/01/28 17:53:07 Joined group=239.255.255.250 on interface=eth1:1900, localIP=10.0.0.2 (listening & sending to port 2021)
2025/01/28 17:53:07 Joined group=239.255.255.250 on interface=eth1:1990, localIP=10.0.0.2 (listening & sending to port 2022)
2025/01/28 17:53:10 Received 17 bytes from 10.0.0.1:1900 on (group=239.255.255.250, iface=eth0, port=1900)
2025/01/28 17:53:10 Forwarded 17 bytes from 10.0.0.1:1900 on iface=eth0 to iface=eth1:2021
```

**Example Systemd Service:**
/etc/systemd/system/ssdp-forwarder.service

```
[Unit]
Description=SSDP Fowards SSDP packets between interfaces
After=network.target

[Service]
TimeoutStartSec=0
Type=simple
User=ssdp-user
ExecStart=/usr/local/bin/ssdp-forwarder -i 'eth1.2,eth1' -p 1990 -d 2021 -g 239.255.255.250
Restart=on-failure
RestartSec=5s
KillMode=process

[Install]
WantedBy=multi-user.target
```
