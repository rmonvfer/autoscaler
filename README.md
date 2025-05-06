# Railway Autoscaler

Pre‑built **horizontal autoscaler** container for [Railway.com](https://railway.com) workloads.

```
# pull the latest image
docker pull ghcr.io/rmonvfer/autoscaler:latest
```

> **Metric:** per‑container CPU %. No code changes required.

## Why
Because surprisingly, a PaaS like Railway does not allow automatic horizontal scaling (that is, increasing the 
replica count for a given service based on load) and instead recommends developers to manually check the load
for each service and then decide if they want to (again, manually) move the replica count slider in the UI.

## How
We use the Railway GraphQL API to poll the CPU usage for each container of your chosen service, aggregate 
them if needed and then decide whether to increase or decrease the number of replicas in your service.
In both cases, the same API is used again to update the replica count.

## Quick start

### 1 – Add the autoscaler image

In Railway **New Service → Deploy from an image** and paste:

```
ghcr.io/rmonvfer/autoscaler:latest
```

(or CLI: `railway add ghcr.io/rmonvfer/autoscaler:latest`)

### 2 – Configure environment variables

```bash
RAILWAY_TOKEN=prj_...        # Project‑Access Token with "Metrics & Deployments" scope
SERVICE_ID=svc_...           # Target service to scale
CPU_HIGH=75  CPU_LOW=30      # thresholds
MIN_REPLICAS=1  MAX_REPLICAS=5
POLL_INTERVAL=30s  COOLDOWN=120s
```

### 3 – Ship it

Keep the autoscaler itself at **one replica**. Capacity now tracks load automatically.

## Build locally (optional)

```bash
docker build -t ghcr.io/rmonvfer/autoscaler:dev .
```

## Design, limitations & roadmap
* Polls Railway GraphQL per‑instance CPU every `POLL_INTERVAL`.
* Scales via `serviceReplicaScale` mutation with hysteresis & cooldown.
* **Limitations:** one target service, CPU only, Railway only.
* **Roadmap:** multiple services, smarter algorithms, ECS/Render/Fly support.

## Contributing
Push a tag (`vX.Y.Z`) on `main`–a GitHub Actions workflow builds a multi‑arch image, pushes to GHCR, and creates a GitHub Release automatically.
