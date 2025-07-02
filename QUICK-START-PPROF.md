# 🚀 Quick Start - pprof Setup Fixed

The issues have been resolved! Here's how to run pprof now:

## ✅ **Fixed Issues:**
1. ✅ Docker build target fixed (`dev` instead of `development`)
2. ✅ pprof support added to main.go
3. ✅ Proper environment variable handling
4. ✅ Easy-to-use scripts created

## 🎯 **Option 1: Run Locally (Recommended for testing)**

```bash
cd /Users/fernando_idwell/go-ai-poi/go-ai-poi-server

# Quick start - runs everything locally
./scripts/run-with-pprof.sh run
```

This will:
- ✅ Check all prerequisites
- ✅ Set up environment variables
- ✅ Start the app with pprof enabled on port 6060
- ✅ Show you all the URLs

## 🐳 **Option 2: Run with Docker (Full monitoring stack)**

```bash
# Start the complete monitoring stack
./scripts/monitoring-setup.sh start
```

This will:
- ✅ Start PostgreSQL, Prometheus, Grafana, Jaeger, etc.
- ✅ Start your Go app with pprof enabled
- ✅ Give you the full observability stack

## 🔬 **Using pprof (Once running)**

### **Quick Commands:**
```bash
# CPU profile (30 seconds)
./scripts/pprof-commands.sh cpu 30

# Memory analysis
./scripts/pprof-commands.sh memory

# Web UI with flame graphs
./scripts/pprof-commands.sh web cpu 8081

# Generate test load
./scripts/pprof-commands.sh load 60 10
```

### **Manual Commands:**
```bash
# CPU flame graph
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile

# Memory heap analysis
go tool pprof -http=:8082 http://localhost:6060/debug/pprof/heap

# Check if pprof is running
curl http://localhost:6060/debug/pprof/
```

## 📊 **Access URLs:**

| Service | URL | Purpose |
|---------|-----|---------|
| **pprof** | http://localhost:6060/debug/pprof/ | Go profiling |
| **API** | http://localhost:8000 | Main application |
| **Metrics** | http://localhost:9090/metrics | Prometheus metrics |
| **Health** | http://localhost:8000/ping | Health check |

## 🔧 **Environment Variables:**

The app now checks for these environment variables:
- `ENABLE_PPROF=true` - Enables pprof server
- `PPROF_PORT=6060` - pprof port (default: 6060)
- `APP_ENV=development` - Application environment

## 🐛 **Troubleshooting:**

### **pprof not working?**
```bash
# Check if app is running with pprof
curl http://localhost:6060/debug/pprof/

# Check logs for pprof startup message
# Should see: "🔬 Starting pprof server"
```

### **Docker build fails?**
```bash
# The Dockerfile now uses the correct 'dev' target
# Make sure you're using the updated docker-compose.monitoring.yml
```

### **App won't start?**
```bash
# Check if PostgreSQL is running
pg_isready -h localhost -p 5454

# Or start with Docker
docker run -d --name postgres-dev -p 5454:5432 -e POSTGRES_DB=go_ai_poi -e POSTGRES_USER=user -e POSTGRES_PASSWORD=password pgvector/pgvector:pg16
```

## 🎯 **Recommended Testing Flow:**

1. **Start locally first:**
   ```bash
   ./scripts/run-with-pprof.sh run
   ```

2. **Generate some load:**
   ```bash
   # In another terminal
   ./scripts/pprof-commands.sh load 30 5
   ```

3. **Profile while load is running:**
   ```bash
   ./scripts/pprof-commands.sh web cpu 8081
   ```

4. **View flame graphs in browser:**
   - Go to: http://localhost:8081
   - Click "Flame Graph" tab
   - Analyze the heat map!

## 🔥 **What You'll See:**

- **CPU Flame Graphs**: Red areas show CPU hotspots
- **Memory Heat Maps**: Shows allocation patterns
- **Goroutine Analysis**: Concurrency usage
- **Real-time Metrics**: In Grafana dashboards

The setup is now working! Try the local option first to verify pprof is working, then move to the full Docker stack for complete monitoring. 🚀