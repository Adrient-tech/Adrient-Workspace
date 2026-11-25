# Adrient Workspace — rendered raw manifests

`adrient.yaml` is the **all-in-one** Kubernetes manifest for the Adrient Workspace
platform: bundled Postgres + rustfs (S3) + Zitadel (OIDC) + the adrient app +
Home Assistant (auto-onboarded, with the Adrient integration), with no external
operators required.

## This file is generated — do not edit by hand

It is rendered from the Helm chart at `../helm/adrient`:

```sh
helm template adrient ../helm/adrient -n adrient \
  --set domain=adrient.example.com \
  --set rustfs.accessKey=adrient --set 'rustfs.secretKey=DevPassword!1' \
  --set homeAssistant.owner.existingSecret=adrient-homeassistant-owner \
  > adrient.yaml
```

To change anything (domain, image tag, credentials, ingress type, enable the
bolo sidecar, swap to external Postgres/OIDC, etc.), edit the chart's
`values.yaml` (or pass `--set`) and re-render. Editing `adrient.yaml` directly
will be lost on the next render.

## Apply with kubectl

The manifest does **not** create the namespace (kept portable). Create it first:

```sh
kubectl create namespace adrient
# Home Assistant's owner account (the manifest references this Secret instead
# of shipping a password that everyone applying this file would share).
kubectl -n adrient create secret generic adrient-homeassistant-owner \
  --from-literal=username=admin \
  --from-literal=password="$(openssl rand -base64 24)"
kubectl apply -n adrient -f adrient.yaml
```

Without that Secret the Home Assistant pod waits (`CreateContainerConfigError`)
and HA is never reachable un-owned. Read the password back with
`kubectl -n adrient get secret adrient-homeassistant-owner -o jsonpath='{.data.password}' | base64 -d`.

Then watch it come up:

```sh
kubectl get pods -n adrient -w
```

Reach adrient via port-forward (the rendered default uses `ingress.type=none`):

```sh
kubectl -n adrient port-forward svc/adrient-adrient 8080:8080
# http://localhost:8080/healthz
```

## Notes / caveats

- **domain** in the rendered file is `adrient.example.com`. Re-render with your
  own `--set domain=...` before applying for real.
- **Credentials are the chart defaults** (admin `admin@adrient.localtest.me` /
  `DevPassword!1`, S3 `adrient` / `DevPassword!1`, Zitadel masterkey). Override
  them via the chart for anything beyond a throwaway/local cluster.
- **Zitadel OIDC auto-provisioning**: the `adrient-zitadel-provision` Job creates
  the adrient OIDC app and writes the `adrient-zitadel-secret` Secret. adrient stays
  `NotReady` until that secret holds a real client-id. Browser-facing SSO login
  needs Zitadel exposed at the same issuer URL adrient uses — see
  `../helm/adrient/README.md` ("Zitadel & real login").
- For production, prefer `helm install` so upgrades and the kept OIDC secret are
  managed; this raw manifest is mainly for clusters without Helm / for review.
