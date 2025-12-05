# streamup

> Stream large files to S3-compatible storage without touching disk.

Production-ready Go library and CLI tool for uploading massive files (1GB to 50TB) to S3-compatible storage using multipart uploads—**without ever saving them to disk**. Perfect for streaming database backups, downloading and re-uploading large datasets, or building data pipelines.

---

## ✨ Features

- 💾 **Zero Disk Usage** — Stream data directly from source to destination
- 🎯 **Constant Memory** — Memory usage stays constant regardless of file size
- 🚀 **Auto-Optimized** — Intelligently calculates optimal part sizes
- 🌍 **Universal** — Works with any S3-compatible service (R2, S3, B2, MinIO, etc.)
- 🛡️ **Production Ready** — Complete error handling, automatic cleanup, progress tracking, retry logic
- 📥 **Flexible Input** — Upload from files, URLs, stdin, or any `io.Reader`
- 📤 **Download Support** — Stream downloads with progress tracking and checksums
- 📋 **Bucket Management** — List objects, cleanup incomplete uploads
- ✅ **Checksum Verification** — MD5 and SHA256 checksums during upload/download
- ⚙️ **Memory Aware** — Optional memory limits for resource-constrained systems
- 📦 **Object Metadata** — Set Content-Type, Cache-Control, custom metadata

---

## 🚀 Quick Start

### Installation

```bash
go install github.com/matthewgall/streamup/cmd/streamup@latest
```

### Basic Usage

```bash
# Set credentials via environment variables
export S3_ACCESS_KEY_ID="your-access-key"
export S3_SECRET_ACCESS_KEY="your-secret-key"
export S3_BUCKET="your-bucket"
export R2_ACCOUNT_ID="your-account-id"  # For Cloudflare R2

# Upload a file
streamup upload backups/database.sql.gz database.sql.gz

# Stream from URL (zero disk usage!)
streamup upload osm/planet.osm.pbf \
  https://planet.openstreetmap.org/pbf/planet-latest.osm.pbf

# Upload with checksum verification
streamup upload backups/data.tar.gz data.tar.gz \
  --checksum --checksum-algorithm sha256

# Download a file
streamup download backups/database.sql.gz database-restored.sql.gz

# List bucket contents
streamup list backups/

# Clean up incomplete uploads
streamup cleanup --prefix backups/
```

### Get Help

```bash
streamup --help                  # Full help with all options
streamup upload --help           # Upload command help
streamup version                 # Show version
streamup version --check-updates # Check for updates
streamup completion bash         # Generate shell completion
```

---

## 💡 Why streamup?

### The Problem: Traditional Workflow

```
Download File → Save to Disk → Upload to S3 → Delete File
├─ Time:   2× file transfer time
├─ Disk:   Full file size required
└─ Memory: Minimal
```

### The Solution: streamup

```
Download → Stream → Upload (simultaneous)
├─ Time:   1× file transfer time (50% faster)
├─ Disk:   0 bytes used
└─ Memory: Constant (~1GB for 70GB file)
```

### Real Example: 70GB OpenStreetMap Planet

**Traditional approach:**
```bash
wget https://planet.openstreetmap.org/pbf/planet-latest.osm.pbf  # 12 min
aws s3 cp planet-latest.osm.pbf s3://bucket/planet.osm.pbf       # 12 min
rm planet-latest.osm.pbf
# Total: ~25 minutes, 70GB disk required
```

**With streamup:**
```bash
streamup upload osm/planet.osm.pbf \
  https://planet.openstreetmap.org/pbf/planet-latest.osm.pbf
# Total: ~12 minutes, 0GB disk, ~1GB RAM
```

**Result:** ⚡ **2× faster**, 💾 **zero disk**, 🎯 **constant memory**

---

## 📊 Performance

| File Size | Part Size | Parts | Memory*  | Upload Time** |
|-----------|-----------|-------|----------|---------------|
| 1 GB      | 5 MB      | 200   | 70 MB    | 10 sec        |
| 10 GB     | 10 MB     | 1000  | 140 MB   | 1.5 min       |
| 70 GB     | 70 MB     | 1000  | 1 GB     | 12 min        |
| 500 GB    | 512 MB    | 1000  | 7 GB     | 1.5 hrs       |
| 5 TB      | 5 GB      | 1000  | 70 GB    | 15 hrs        |

<sub>\* Memory = partSize × (workers + queueSize), defaults: workers=4, queue=10</sub>
<sub>\*\* Assuming 100 MB/s network speed</sub>

---

## 🌐 S3-Compatible Services

Works with **any** S3-compatible storage:

<table>
<tr>
<td width="50%">

**Cloudflare R2** (default)
```bash
streamup upload data.zip data.zip
```

**AWS S3**
```bash
streamup upload data.zip data.zip \
  --endpoint s3.amazonaws.com \
  --region us-west-2
```

</td>
<td width="50%">

**Backblaze B2**
```bash
streamup upload data.zip data.zip \
  --endpoint s3.us-west-002.backblazeb2.com
```

**MinIO / Others**
```bash
streamup upload data.zip data.zip \
  --endpoint minio.example.com:9000
```

</td>
</tr>
</table>

---

## 📚 Common Use Cases

### Database Backups

```bash
# PostgreSQL → R2 with checksum
pg_dump mydb | gzip | streamup upload backups/db-$(date +%Y%m%d).sql.gz - \
  --size 50000000000 --checksum-algorithm sha256

# MySQL → S3
mysqldump --all-databases | gzip | streamup upload mysql/$(date +%Y%m%d).sql.gz - \
  --size 25000000000
```

### Large Datasets

```bash
# Download & upload research data (no intermediate file)
streamup upload datasets/research.tar.gz https://data.example.com/dataset.tar.gz

# Download from S3 with checksum verification
streamup download datasets/research.tar.gz research.tar.gz --checksum
```

### Log Archival

```bash
# Archive logs directly to S3
tar czf - /var/log | streamup upload logs/archive-$(date +%Y%m%d).tar.gz - \
  --size 10000000000

# List old archives
streamup list logs/
```

### Media Pipelines

```bash
# Transcode video and upload in one stream with metadata
ffmpeg -i input.mp4 -f mp4 - | streamup upload videos/output.mp4 - \
  --size 5000000000 \
  --content-type video/mp4 \
  --metadata "title=My Video"
```

---

## 🔧 Configuration

### Environment Variables

```bash
S3_ACCESS_KEY_ID       # S3 access key (required)
S3_SECRET_ACCESS_KEY   # S3 secret key (required)
S3_BUCKET              # S3 bucket name (required)
S3_ENDPOINT            # Custom endpoint (optional)
S3_REGION              # S3 region (optional)
R2_ACCOUNT_ID          # Cloudflare R2 account ID (R2 only)
```

### CLI Commands

Run `streamup --help` to see all available commands and options:

**Commands:**
- `upload <key> <source>` — Upload a file to S3
- `download <key> <destination>` — Download a file from S3
- `list [prefix]` — List objects in S3 bucket
- `cleanup` — Clean up incomplete multipart uploads
- `version` — Show version information
- `completion` — Generate shell completion scripts

**Upload Options:**
- **Input**: File path, URL, or `-` for stdin with `--size`
- **Checksum**: `--checksum`, `--checksum-algorithm` (md5/sha256)
- **Metadata**: `--content-type`, `--cache-control`, `--metadata key=value`
- **Performance**: `--workers`, `--queue`, `--max-memory`
- **Retry**: `--max-retries`, `--retry-delay`, `--max-retry-delay`
- **Service**: `--endpoint`, `--region`, `--account-id`
- **Advanced**: `--min-part-size`, `--max-part-size`, `--max-parts`
- **Output**: `--quiet`

### Shell Completion

```bash
streamup completion bash > /etc/bash_completion.d/streamup
streamup completion zsh > ~/.zsh/completion/_streamup
streamup completion fish > ~/.config/fish/completions/streamup.fish
```

---

## 💻 Library Usage

### Simple Upload

```go
package main

import (
    "os"
    "github.com/matthewgall/streamup/pkg/streamup"
)

func main() {
    file, _ := os.Open("large-file.dat")
    defer file.Close()
    stat, _ := file.Stat()

    cfg := streamup.Config{
        AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
        SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
        Bucket:          os.Getenv("S3_BUCKET"),
        Key:             "uploads/large-file.dat",
        FileSize:        stat.Size(),
        AccountID:       os.Getenv("R2_ACCOUNT_ID"), // For R2
    }

    uploader, _ := streamup.New(cfg)
    uploader.Upload(file)
}
```

### With Progress Tracking

```go
cfg := streamup.Config{
    // ... other config ...
    ProgressCallback: func(bytesUploaded int64, partsUploaded int32) {
        pct := float64(bytesUploaded) / float64(fileSize) * 100
        fmt.Printf("Progress: %.1f%% (%d parts)\n", pct, partsUploaded)
    },
}
```

### Stream from HTTP

```go
resp, _ := http.Get("https://example.com/file.dat")
defer resp.Body.Close()

cfg := streamup.Config{
    // ... credentials ...
    Key:      "downloads/file.dat",
    FileSize: resp.ContentLength,
}

uploader, _ := streamup.New(cfg)
uploader.Upload(resp.Body)
```

---

## 🧠 How It Works

**Producer-Consumer Pattern with Worker Pool:**

```
Reader → Producer → [Queue] → Workers (×4) → S3
                       ↓
                  Collector → Complete
```

1. **Producer** reads data chunks from `io.Reader`
2. **Queue** buffers parts (configurable size)
3. **Workers** upload parts concurrently to S3
4. **Collector** gathers ETags and tracks progress
5. **Complete** sends final multipart completion to S3

**Memory Formula:** `partSize × (workers + queueSize) = constant RAM`

**Part Size Algorithm:**
- Targets ~1000 parts for optimal performance
- Respects service limits (5MB-5GB per part, max 10K parts)
- Adjusts for memory constraints if specified
- Rounds to nearest MB for clean numbers

---

## 🐛 Troubleshooting

| Error | Solution |
|-------|----------|
| `credentials required` | Set `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_BUCKET` |
| `file size exceeds limit` | File >50TB requires custom service limits |
| `upload error` | Network issue—upload auto-aborted, check connection |
| High memory usage | Use `--max-memory 2048` to limit to 2GB |

---

## 📖 Documentation

- Run `streamup --help` for full CLI documentation
- See [QUICKSTART.md](QUICKSTART.md) for a 5-minute guide
- Check [pkg/streamup](pkg/streamup) for library documentation

---

## 🤝 Contributing

Contributions welcome! Please open an issue or PR.

## 📄 License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

Copyright 2025 Matthew Gall

## 🙏 Credits

Built with [AWS SDK for Go v2](https://github.com/aws/aws-sdk-go-v2) and [Cobra](https://github.com/spf13/cobra).
