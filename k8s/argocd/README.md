# ArgoCD + Image Updater

This directory contains the ArgoCD `Application` for prod, configured for
ArgoCD Image Updater to track immutable GHCR tags (`sha-<40 hex>`).

## Apply the Application

```bash
kubectl apply -k k8s/argocd
```

## Required credentials

If GHCR packages are private, create two pull secrets:

1. `argocd/ghcr-pullsecret` for ArgoCD Image Updater registry access.
2. `game-prod/ghcr-pullsecret` for workload image pulls at runtime.

Use a token with at least `read:packages`.

```bash
kubectl -n argocd create secret docker-registry ghcr-pullsecret \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<github-token>

kubectl -n game-prod create secret docker-registry ghcr-pullsecret \
  --docker-server=ghcr.io \
  --docker-username=<github-username> \
  --docker-password=<github-token>
```

The prod overlay configures the default service account to use
`ghcr-pullsecret` for image pulls.
