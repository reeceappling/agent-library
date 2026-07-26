---
name: docker-security-auditor
description: Automates security scanning, vulnerability identification, and compliance patching for local Docker environments. Trigger this when analyzing Dockerfiles or inspecting running containers for vulnerabilities.
variables:
  target_image:
    type: string
    description: "The name or ID of the Docker image to scan."
    required: true
  severity_threshold:
    type: string
    description: "Minimum vulnerability level to report (LOW, MEDIUM, HIGH, CRITICAL)."
    default: "HIGH"
---

# Docker Security Auditor Skill

Use this skill to audit Docker containers, parse vulnerability databases, and apply automated hardening patches.

## 1. Prerequisites & Environment Check
Before executing, verify the local environment has the required scanning binaries:
* Ensure the Docker daemon is actively running: `docker info`
* Verify the security scanner is installed: `trivy --version`
* If `trivy` is missing, abort and prompt the user to install it via `brew install aquasecurity/trivy/trivy`.

## 2. Execution Workflow

### Step 1: Image Vulnerability Scan
Execute a structured JSON scan on the target image filtered by the severity threshold. Run this command exactly:
```bash
trivy image --severity {{severity_threshold}} --format json --output audit-report.json {{target_image}}
```

### Step 2: Static Dockerfile Analysis
If a `Dockerfile` exists in the current root workspace, run a configuration audit to detect root-user violations or unpinned base tags:
```bash
trivy config --severity HIGH,CRITICAL .
```

### Step 3: Run-Time Inspection
If the image is already running as a live container, inspect its security profile and cap-add privileges:
```bash
docker inspect --format='{{json .HostConfig.Capabilities}}' {{target_image}}
```

## 3. Error Handling & Troubleshooting
* **Error: "Conn refused to docker daemon"**
    * Action: Start the Docker Desktop application or execute `sudo systemctl start docker`.
* **Error: "Trivy database outdated"**
    * Action: Force a database sync by running `trivy image --download-db-only`.

## 4. Required Output Format
Synthesize the `audit-report.json` data into a clean markdown table. The final output to the user **must** strictly mirror this structure:

### 🛡️ Security Audit Report: {{target_image}}

| Library / Package | Current Version | Fixed Version | Severity | Vulnerability ID |
| :--- | :--- | :--- | :--- | :--- |
| `openssl` | 1.1.1t-r0 | 1.1.1u-r0 | CRITICAL | CVE-2023-0464 |
| `libcrypto1.1` | 1.1.1t-r0 | 1.1.1u-r0 | HIGH | CVE-2023-0465 |

### 🛠️ Remediation Steps
1. **Base Image Upgrade**: Update the first line of the `Dockerfile` to use the patched version.
2. **Package Patching**: Add `RUN apk upgrade --no-cache` to force downstream updates during the build phase.
