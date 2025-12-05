# streamup Quick Start

> Get up and running with streamup in 5 minutes.

---

## 📦 Installation

```bash
go install github.com/matthewgall/streamup/cmd/streamup@latest
```

**Or build from source:**

```bash
git clone https://github.com/matthewgall/streamup
cd streamup
go build -o streamup ./cmd/streamup
```

---

## 🔑 Setup Credentials

### Environment Variables (Recommended)

```bash
# For Cloudflare R2
export S3_ACCESS_KEY_ID="your-access-key-id"
export S3_SECRET_ACCESS_KEY="your-secret-access-key"
export S3_BUCKET="your-bucket-name"
export R2_ACCOUNT_ID="your-account-id"

# For AWS S3
export S3_ACCESS_KEY_ID="your-aws-access-key"
export S3_SECRET_ACCESS_KEY="your-aws-secret-key"
export S3_BUCKET="your-bucket-name"
export S3_ENDPOINT="s3.amazonaws.com"
export S3_REGION="us-west-2"
```

### Or use flags

```bash
streamup -k path/to/file.dat -f local-file.dat \
  --access-key "your-key" \
  --secret-key "your-secret" \
  --bucket "your-bucket"
```

---

## 🚀 Basic Usage

### Upload a Local File

```bash
# Upload to R2 (simplest)
streamup -k uploads/myfile.zip -f myfile.zip

# Upload to AWS S3
streamup -k uploads/myfile.zip -f myfile.zip \
  --endpoint s3.amazonaws.com \
  --region us-west-2
```

### Stream from URL (Zero Disk!)

```bash
# Download and upload simultaneously - no intermediate file
streamup -k datasets/large-dataset.tar.gz \
  -u https://example.com/large-dataset.tar.gz
```

### Upload from stdin

```bash
# Database backup
pg_dump mydb | gzip | streamup -k backups/mydb.sql.gz -s 50000000000

# Archive directory
tar czf - /path/to/data | streamup -k archives/data.tar.gz -s 10000000000
```

> **Note:** When using stdin, you must specify `-s/--size` with the expected file size in bytes.

---

## 💡 Common Use Cases

### 🗄️ Database Backups

<table>
<tr>
<td width="50%">

**PostgreSQL**
```bash
pg_dump -Fc mydb | streamup \
  -k backups/postgres/mydb-$(date +%Y%m%d).dump \
  -s 75000000000
```

</td>
<td width="50%">

**MySQL**
```bash
mysqldump --all-databases | gzip | streamup \
  -k backups/mysql/all-$(date +%Y%m%d).sql.gz \
  -s 25000000000
```

</td>
</tr>
</table>

### 🌍 OpenStreetMap Planet

```bash
# Download 70GB OSM planet and upload to R2
# ⚡ Time: ~12 min | 💾 Disk: 0 GB | 🧠 Memory: ~1 GB
streamup -k osm/planet-latest.osm.pbf \
  -u https://planet.openstreetmap.org/pbf/planet-latest.osm.pbf
```

### 📝 Log Archival

```bash
# Archive logs from last 30 days
find /var/log -name "*.log" -mtime -30 | \
  tar czf - -T - | \
  streamup -k logs/archive-$(date +%Y%m%d).tar.gz -s 5000000000
```

### 🎬 Media Processing

```bash
# Upload video
streamup -k media/videos/video.mp4 -f video.mp4

# Transcode and upload (no intermediate file)
ffmpeg -i input.avi -f mp4 - | \
  streamup -k media/output.mp4 -s 8000000000
```

### 🖥️ Memory-Constrained Systems

```bash
# Limit memory to 2GB on small VPS
streamup -k backups/large.tar.gz -f large.tar.gz --max-memory 2048
```

---

## ⚙️ Configuration Tips

### Performance Tuning

```bash
# More workers = faster (uses more memory)
streamup -k large.dat -f large.dat -w 8

# Larger queue = better throughput
streamup -k large.dat -f large.dat --queue 20
```

### Memory Limits

```bash
# Strict 1GB memory limit
streamup -k 500gb.dat -f 500gb.dat --max-memory 1024
# Result: Uses 1GB RAM with ~4918 parts of ~107 MB each
```

### Custom Service Limits

```bash
# For non-standard S3 service
streamup -k data.dat -f data.dat \
  --endpoint custom.s3.example.com \
  --min-part-size 10485760 \      # 10 MB
  --max-part-size 1073741824 \    # 1 GB
  --max-parts 5000
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
    file, _ := os.Open("largefile.dat")
    defer file.Close()
    stat, _ := file.Stat()

    cfg := streamup.Config{
        AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
        SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
        Bucket:          os.Getenv("S3_BUCKET"),
        Key:             "uploads/largefile.dat",
        FileSize:        stat.Size(),
        AccountID:       os.Getenv("R2_ACCOUNT_ID"),
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
package main

import (
    "net/http"
    "github.com/matthewgall/streamup/pkg/streamup"
)

func main() {
    resp, _ := http.Get("https://example.com/largefile.dat")
    defer resp.Body.Close()

    cfg := streamup.Config{
        // ... credentials ...
        Key:      "downloads/largefile.dat",
        FileSize: resp.ContentLength,
    }

    uploader, _ := streamup.New(cfg)
    uploader.Upload(resp.Body)
}
```

---

## 🐛 Troubleshooting

| Problem | Solution |
|---------|----------|
| `validation error for AccessKeyID` | Set `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_BUCKET` |
| `upload error during CreateMultipartUpload` | Check credentials, bucket exists, correct endpoint/region |
| High memory usage | Use `--max-memory 2048` to limit to 2GB |
| `server did not provide Content-Length` | Server must provide header, or use `-f` instead of `-u` |
| Upload is slow | Try `-w 8` (more workers), `--queue 20` (larger queue) |

---

## 📚 Next Steps

- **Full Documentation:** Read [README.md](README.md) for complete docs
- **Shell Completion:** `streamup completion bash > /etc/bash_completion.d/streamup`
- **All Options:** Run `streamup --help`

## 🆘 Getting Help

- **CLI Help:** `streamup --help`
- **Documentation:** [README.md](README.md)
- **Issues:** Open an issue on GitHub

---

**Happy Streaming! 🚀**
