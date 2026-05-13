#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

PLATFORM="${PLATFORM:-linux/amd64}"
IMAGE_REPOSITORY="${IMAGE_REPOSITORY:-aiops-v2}"
GIT_SHA="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_DATE="$(date +%Y%m%d)"
WORKTREE_STATE="unknown"
DIRTY_SUFFIX=""
if git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  if git -C "$ROOT_DIR" status --porcelain | grep -q .; then
    WORKTREE_STATE="dirty"
    DIRTY_SUFFIX="-dirty"
  else
    WORKTREE_STATE="clean"
  fi
fi
IMAGE_TAG="${IMAGE_TAG:-offline-${BUILD_DATE}-${GIT_SHA}${DIRTY_SUFFIX}}"
IMAGE_REF="${IMAGE_REPOSITORY}:${IMAGE_TAG}"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/output}"
SKIP_DOCKER_BUILD="${SKIP_DOCKER_BUILD:-0}"

case "$PLATFORM" in
  linux/*) ;;
  *)
    echo "PLATFORM must be linux/<arch>, got: $PLATFORM" >&2
    exit 1
    ;;
esac

ARCH="${PLATFORM#linux/}"
SAFE_IMAGE_REF="$(printf '%s' "$IMAGE_REF" | tr '/:' '__')"
BUNDLE_TAG="$(printf '%s' "$IMAGE_TAG" | tr '/:' '__')"
BUNDLE_NAME="aiops-v2-${BUNDLE_TAG}-${ARCH}"
BUNDLE_DIR="${OUTPUT_DIR%/}/${BUNDLE_NAME}"
IMAGE_ARCHIVE_NAME="${SAFE_IMAGE_REF}_${ARCH}.tar.gz"
IMAGE_ARCHIVE_PATH="$BUNDLE_DIR/images/$IMAGE_ARCHIVE_NAME"

sha256_one() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1"
  else
    shasum -a 256 "$1"
  fi
}

rm -rf "$BUNDLE_DIR"
mkdir -p "$BUNDLE_DIR/images" "$BUNDLE_DIR/kubernetes" "$BUNDLE_DIR/scripts"

if [ "$SKIP_DOCKER_BUILD" != "1" ]; then
  docker buildx build \
    --platform "$PLATFORM" \
    -t "$IMAGE_REF" \
    --load \
    "$ROOT_DIR"
fi

docker save "$IMAGE_REF" | gzip -c > "$IMAGE_ARCHIVE_PATH"

awk -v image="$IMAGE_REF" '
  $1 == "image:" && $2 ~ /aiops-v2:/ {
    sub(/image:.*/, "image: " image)
  }
  { print }
' "$ROOT_DIR/deploy/k8s/aiops-v2.yaml" > "$BUNDLE_DIR/kubernetes/aiops-v2.yaml"

cp "$ROOT_DIR/deploy/k8s/aiops-v2-ingress.example.yaml" "$BUNDLE_DIR/kubernetes/aiops-v2-ingress.example.yaml"

cat > "$BUNDLE_DIR/scripts/load-image.sh" <<EOF
#!/usr/bin/env sh
set -eu

BUNDLE_DIR="\$(CDPATH= cd -- "\$(dirname -- "\$0")/.." && pwd)"
IMAGE_ARCHIVE="\$BUNDLE_DIR/images/$IMAGE_ARCHIVE_NAME"
RUNTIME="\${1:-docker}"

case "\$RUNTIME" in
  docker)
    gzip -dc "\$IMAGE_ARCHIVE" | docker load
    ;;
  containerd|ctr)
    tmp="\${TMPDIR:-/tmp}/aiops-v2-image-\$\$.tar"
    trap 'rm -f "\$tmp"' EXIT
    gzip -dc "\$IMAGE_ARCHIVE" > "\$tmp"
    ctr -n k8s.io images import "\$tmp"
    ;;
  nerdctl)
    tmp="\${TMPDIR:-/tmp}/aiops-v2-image-\$\$.tar"
    trap 'rm -f "\$tmp"' EXIT
    gzip -dc "\$IMAGE_ARCHIVE" > "\$tmp"
    nerdctl -n k8s.io load -i "\$tmp"
    ;;
  *)
    echo "Usage: \$0 [docker|containerd|ctr|nerdctl]" >&2
    exit 1
    ;;
esac

echo "Loaded image: $IMAGE_REF"
EOF

cat > "$BUNDLE_DIR/scripts/push-to-registry.sh" <<EOF
#!/usr/bin/env sh
set -eu

if [ "\${1:-}" = "" ]; then
  echo "Usage: \$0 <registry-or-registry/namespace>" >&2
  echo "Example: \$0 registry.intra.example.com/aiops" >&2
  exit 1
fi

BUNDLE_DIR="\$(CDPATH= cd -- "\$(dirname -- "\$0")/.." && pwd)"
REGISTRY="\${1%/}"
SOURCE_IMAGE="$IMAGE_REF"
TARGET_IMAGE="\$REGISTRY/aiops-v2:$IMAGE_TAG"

gzip -dc "\$BUNDLE_DIR/images/$IMAGE_ARCHIVE_NAME" | docker load
docker tag "\$SOURCE_IMAGE" "\$TARGET_IMAGE"
docker push "\$TARGET_IMAGE"

echo "Pushed image: \$TARGET_IMAGE"
echo "Use this in kubernetes/aiops-v2.yaml:"
echo "  image: \$TARGET_IMAGE"
EOF

chmod +x "$BUNDLE_DIR/scripts/load-image.sh" "$BUNDLE_DIR/scripts/push-to-registry.sh"

cat > "$BUNDLE_DIR/README.md" <<EOF
# aiops-v2 offline bundle

Build metadata:

- Image: \`$IMAGE_REF\`
- Platform: \`$PLATFORM\`
- Source commit: \`$GIT_SHA\`
- Working tree: \`$WORKTREE_STATE\`
- Build date: \`$BUILD_DATE\`

## Files

- \`images/$IMAGE_ARCHIVE_NAME\`: image archive produced by \`docker save | gzip\`
- \`kubernetes/aiops-v2.yaml\`: namespace, config, secret placeholder, PVC, Deployment, Service
- \`kubernetes/aiops-v2-ingress.example.yaml\`: optional Ingress example
- \`scripts/load-image.sh\`: import image into Docker/containerd/nerdctl
- \`scripts/push-to-registry.sh\`: load, retag, and push to an internal registry

## Option A: internal registry

Run this on a machine that can reach the internal registry:

\`\`\`sh
./scripts/push-to-registry.sh registry.intra.example.com/aiops
\`\`\`

Then replace the image in \`kubernetes/aiops-v2.yaml\` with the printed registry image, edit the Secret placeholders, and deploy:

\`\`\`sh
kubectl apply -f kubernetes/aiops-v2.yaml
kubectl -n aiops get pods
\`\`\`

## Option B: no internal registry

Copy this bundle to every Kubernetes node and import the image into the node runtime:

\`\`\`sh
# Docker runtime
./scripts/load-image.sh docker

# containerd runtime, commonly used by Kubernetes
sudo ./scripts/load-image.sh containerd

# nerdctl-managed containerd
sudo ./scripts/load-image.sh nerdctl
\`\`\`

Keep \`image: $IMAGE_REF\` and \`imagePullPolicy: IfNotPresent\` in \`kubernetes/aiops-v2.yaml\`, edit the Secret placeholders, then deploy:

\`\`\`sh
kubectl apply -f kubernetes/aiops-v2.yaml
kubectl -n aiops rollout status deploy/aiops-v2
kubectl -n aiops port-forward svc/aiops-v2 8080:80
\`\`\`

Open \`http://127.0.0.1:8080\`.

## Required configuration

Before applying, replace these placeholders in \`kubernetes/aiops-v2.yaml\`:

- \`AIOPS_LLM_API_KEY\`
- \`AIOPS_ACTION_TOKEN_SECRET\`

For an OpenAI-compatible intranet gateway, also set \`AIOPS_LLM_BASE_URL\` in the ConfigMap.
EOF

(
  cd "$BUNDLE_DIR"
  sha256_one "images/$IMAGE_ARCHIVE_NAME" > SHA256SUMS
  sha256_one "kubernetes/aiops-v2.yaml" >> SHA256SUMS
  sha256_one "kubernetes/aiops-v2-ingress.example.yaml" >> SHA256SUMS
)

tar -czf "${OUTPUT_DIR%/}/${BUNDLE_NAME}.tar.gz" -C "$OUTPUT_DIR" "$BUNDLE_NAME"

echo "Bundle directory: $BUNDLE_DIR"
echo "Bundle archive: ${OUTPUT_DIR%/}/${BUNDLE_NAME}.tar.gz"
echo "Image archive: $IMAGE_ARCHIVE_PATH"
