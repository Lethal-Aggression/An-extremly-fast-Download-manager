# TURBODL
### Multi-Interface Hardware Bandwidth Aggregator & Download Accelerator

```text
┌──────────────────────────────────────────────────────────────────────┐
│  TURBODL // HARDWARE SPEED BONDING ENGINE v1.0.0                      │
│  TARGET: ubuntu-24.04-desktop-amd64.iso                   DOWNLOADING│
├──────────────────────────────────────────────────────────────────────┤
│   AGGREGATED BANDWIDTH:   ▶  35.8 MB/s  ◀                            │
├──────────────────────────────────────────────────────────────────────┤
│  INTERFACE                    LOCAL IP               SPEED           │
│   [WIFI]  PC WI-FI  [wlp3s0]  192.168.1.20     ██████████████ 20.6 MB/s│
│   [PHONE] TETHER    [usb0]    192.168.42.12    ██████████░░░░ 15.2 MB/s│
├──────────────────────────────────────────────────────────────────────┤
│  PROGRESS: [████████████████████████░░░░░░░░░░░░░░░░░░░░░░]   54.2%  │
│   TRANSFERRED: 2.94 GB / 5.42 GB      ETA: 01m 09s    CHUNKS: 342/650│
├──────────────────────────────────────────────────────────────────────┤
│  [+] [HOT-PLUG] PHONE DETECTED (usb0) -> 4 THREADS INJECTED!         │
└──────────────────────────────────────────────────────────────────────┘
 [CTRL+C] QUIT / DETACH    [P] PAUSE/RESUME    [HOT-PLUG ACTIVE]
```

TurboDL is an ultra-minimalist, high-performance download manager designed for pure speed. It solves the classic single-gateway bottleneck by combining your **PC Wi-Fi** and **Phone USB Tethering** simultaneously into a single, aggregated multi-threaded download stream.

If your PC Wi-Fi reaches **20 MiB/s** and your Phone reaches **15 MiB/s**, TurboDL bonds them at the socket level so your download hits **35 MiB/s**.

---

## FEATURES

- **Hardware Bandwidth Aggregation**: Bypasses operating system single-gateway routing by binding individual HTTP worker sockets directly to specific network adapters (`SO_BINDTODEVICE` on Linux and `IP_UNICAST_IF` / `LocalAddr` on Windows).
- **1-Click Phone Hot-Plug Detection**: Plug your phone in via USB cable and turn on USB Tethering. TurboDL detects the new interface, verifies route connectivity, and injects worker threads into active downloads in real-time.
- **Dynamic Work-Stealing Range Engine**: Automatically balances workload based on interface speed. Faster connections naturally complete more chunks without bottlenecking slower connections.
- **Pitch-Black & White Minimalism**: Sharp Unicode box-drawing borders, high-contrast monospace telemetry, zero web browser overhead, and instant 2ms startup time.
- **Zero-Copy Sparse Storage**: Pre-allocates files and writes directly to disk offsets (`WriteAt`). No slow concatenation or merging at the end of large downloads.
- **Fault-Tolerant Unplug Protection**: If your phone cable gets disconnected mid-download, the engine revokes the adapter, recycles interrupted chunks, and finishes seamlessly on PC Wi-Fi without corrupting the file.
- **Cross-Platform**: Compiles to standalone single binaries for Windows (`turbodl.exe`) and Linux (`turbodl`).

---

## QUICKSTART

### Windows (PowerShell)
Run with a single command in PowerShell:
```powershell
powershell -ExecutionPolicy Bypass -File turbodl.ps1
```
Or launch directly with a download URL:
```powershell
.\turbodl.exe https://example.com/large-file.iso
```

### Linux
```bash
./turbodl.sh
```
Or launch directly with a download URL:
```bash
./turbodl https://example.com/large-file.iso
```

---

## PHONE BONDING SETUP

1. Connect your phone to your PC via a USB cable.
2. Enable USB Tethering:
   - **Android**: Go to **Settings** -> **Network & Internet** -> **Hotspot & Tethering** -> enable **USB Tethering**.
   - **iPhone**: Go to **Settings** -> **Personal Hotspot** -> enable **Allow Others to Join** (USB tethering activates automatically).
3. Run TurboDL:
   - TurboDL detects the tethered adapter (`RNDIS`, `usb0`, or `Apple Mobile Device Ethernet`).
   - The interface is added to the bonding pool. Both PC Wi-Fi and Phone network pipes download concurrently.

> Tip: To reach maximum throughput (e.g. 20 + 15 = 35 MB/s), connect your PC to your home Wi-Fi and have your phone use 4G/5G mobile data (or connect your phone to a 5 GHz Wi-Fi band while your PC is on 2.4 GHz).

---

## BUILDING FROM SOURCE

Prerequisites: Go 1.21+

```bash
git clone https://github.com/Lethal-Aggression/An-extremly-fast-Download-manager.git
cd An-extremly-fast-Download-manager

# Build both Linux binary and Windows .exe:
make build

# Or build individually:
make linux    # Outputs ./turbodl
make windows  # Outputs ./turbodl.exe
```

---

## CONFIGURATION (turbodl_config.json)

```json
{
  "download_dir": "./downloads",
  "chunk_size_mb": 8,
  "threads_per_adapter": 4,
  "auto_bond_all": true,
  "web_port": 8989,
  "remembered_phones": {}
}
```

- `download_dir`: Destination folder for completed files.
- `chunk_size_mb`: Size of each parallel HTTP Range block (default: 8 MB).
- `threads_per_adapter`: Concurrent TCP streams per network card (default: 4).
- `auto_bond_all`: Automatically bond every internet-connected interface upon detection.

---

## ARCHITECTURE

```text
┌────────────────────────────────────────────────────────┐
│                      TURBODL CORE                      │
│             [Adaptive Chunk Work-Stealing]             │
└───────────────┬────────────────────────┬───────────────┘
                │                        │
       [4 Worker Threads]       [4 Worker Threads]
                │                        │
        SO_BINDTODEVICE            SO_BINDTODEVICE
        / IP_UNICAST_IF            / IP_UNICAST_IF
                │                        │
     ┌──────────▼──────────┐  ┌──────────▼──────────┐
     │  PC WI-FI ADAPTER   │  │  PHONE USB TETHER   │
     │      (wlp3s0)       │  │       (usb0)        │
     │      20.6 MB/s      │  │     15.2 MB/s       │
     └──────────┬──────────┘  └──────────┬──────────┘
                │                        │
                └───────────┬────────────┘
                            │
              Direct WriteAt (Offset)
                            ▼
               [Pre-allocated Target File]
                  (Instant 0ms Finish)
```

---

## LICENSE
MIT License.
