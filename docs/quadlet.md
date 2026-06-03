# Quadlet デプロイ

`compose.yaml` を正とします。`quadlet/` のファイルは Compose 定義を Podman Quadlet 向けに写したものです。`systemd/` のファイルは Quadlet ではなく、通常の systemd unit です。

Quadlet は rootful system unit として動かします。ただし container process は host root と同一 UID ではなく、固定の user namespace mapping を使います。

- `findsenryu-data.volume`: 永続データ用の Podman named volume を作成します。
- `findsenryu.image`: OCIR image を root 管理の authfile で pull します。
- `findsenryu-init.container`: `data/backups` を作成し、distroless nonroot UID 用に所有者を調整します。
- `findsenryu-migrate.container`: `init` 完了後に `/app/migrate` を実行します。
- `findsenryu-app.container`: `migrate` 完了後に bot を起動します。
- `systemd/findsenryu4discord.target`: スタック全体の運用単位です。

`[Container]` / `[Volume]` の基本部分は `podlet 0.3.2` の `podlet podman run` / `podlet podman volume create` で生成しました。`--uidmap` / `--gidmap` / `:U` の変換も `podlet 0.3.2` で確認しています。Compose の named volume 名と合わせるための `VolumeName=findsenryu-data`、OCIR pull 用の `.image` unit、container 側の `Image=findsenryu.image`、systemd target は Podlet 生成後に手動で保持しています。

WSL 内の `podlet compose compose.yaml` も再検証しました。`podlet` は Compose の `${FINDSENRYU_IMAGE}` を展開しないため、検証時は `envsubst` で image を展開して渡しました。その結果、volume/path ではなく `service_completed_successfully` が直接サポートされない点で止まります。`.target` は Quadlet ではないため、`podlet compose` が通る場合でも別途 systemd unit として配置します。

Compose の `init -> migrate -> app` の順序、`restart: always`、`stop_grace_period` に対応する target 構成と `[Unit]` / `[Service]` は手動で保持しています。

`findsenryu-init.container` と `findsenryu-migrate.container` は有限の準備処理なので、`Type=oneshot` と `RemainAfterExit=yes` を使います。`findsenryu-app.container` は常駐コンテナなので Quadlet の既定 service type に任せます。

## User Namespace

各 container は同じ mapping を使います。

```text
UIDMap=0:100000:65536
GIDMap=0:100000:65536
```

これにより container UID/GID `0..65535` は host UID/GID `100000..165535` に対応します。distroless `nonroot` の UID/GID `65532` は host 側では `165532` です。

`UserNS=auto` は使いません。`auto` は container ごとに異なる range を割り当てるため、`init` が chown した named volume を `migrate` / `app` が同じ owner として扱えない可能性があります。固定 mapping にすることで Compose の `init -> migrate -> app` と named volume の前提を保ちます。

`findsenryu-init.container` の data volume だけ `:U` を付けています。これは user namespace 上の root が初回 volume layout を作成し、その後 Compose と同じ `chown -R 65532:65532 /app/data` を実行できるようにするためです。`migrate` / `app` は同じ mapping で同じ named volume を参照します。

この host UID/GID range を変える場合は、`findsenryu-init.container`、`findsenryu-migrate.container`、`findsenryu-app.container` の `UIDMap` / `GIDMap` を同時に変更してください。既存の `findsenryu-data` volume がある場合は、host 側 owner も新しい mapping に合わせる必要があります。

## インストール

管理対象ファイルでは config 配置先を次にしています。

```text
/opt/findsenryu4discord
```

Podman ホスト上にこのディレクトリを作成し、`config.toml` を置きます。永続データは `findsenryu-data` named volume に保存されます。

```bash
sudo install -d -m 0755 /opt/findsenryu4discord
sudo install -o 165532 -g 165532 -m 0400 config.toml /opt/findsenryu4discord/config.toml
```

`165532` は、この手順の `UIDMap=0:100000:65536` / `GIDMap=0:100000:65536` で container UID/GID `65532` に対応する host UID/GID です。`config.toml` は bind mount なので、host root 所有 `0600` では `app` / `migrate` の実行 UID から読めません。

OCIR イメージの placeholder を置換します。

```bash
image="<ocir-image-uri>"
sed -i "s#<ocir-image-uri>#$image#g" \
  quadlet/findsenryu.image
```

OCIR に rootful Podman として login します。rootful systemd unit から読むため、authfile は `/etc/containers/findsenryu4discord/auth.json` に固定します。

```bash
export OCIR_REGISTRY="<ocir-registry>"
export OCIR_USERNAME="<tenancy-namespace>/<oci-username>"

sudo install -d -m 0700 /etc/containers/findsenryu4discord
sudo podman login --authfile /etc/containers/findsenryu4discord/auth.json "$OCIR_REGISTRY" -u "$OCIR_USERNAME"
sudo chown root:root /etc/containers/findsenryu4discord/auth.json
sudo chmod 600 /etc/containers/findsenryu4discord/auth.json
```

`OCIR_USERNAME` は通常 `<tenancy-namespace>/<username>` です。フェデレートされたユーザーの場合は `<tenancy-namespace>/<domain-name>/<username>` です。password には OCI Auth Token を入力してください。

Quadlet と target を install します。`findsenryu4discord.target` は Quadlet file ではないため、`podman quadlet install --replace quadlet/` では配置されません。

```bash
sudo podman quadlet install --replace quadlet/
sudo install -D -m 0644 systemd/findsenryu4discord.target /etc/systemd/system/findsenryu4discord.target
sudo systemctl daemon-reload
sudo systemctl enable --now findsenryu4discord.target
```

`sudo podman quadlet install --replace quadlet/` の出力先は rootful Quadlet search path である `/etc/containers/systemd/` であることを確認してください。`findsenryu.image` は `AuthFile=/etc/containers/findsenryu4discord/auth.json` と `Policy=always` を指定しています。`findsenryu-migrate.container` と `findsenryu-app.container` は `Image=findsenryu.image` を参照するため、起動時の pull は `findsenryu-image.service` 経由で実行されます。

ログ確認:

```bash
sudo journalctl -u findsenryu-app.service -f
```

イメージや Quadlet file を更新した場合は target を restart します。これにより `init` と `migrate` が実行されてから `app` が起動します。

```bash
sudo podman quadlet install --replace quadlet/
sudo install -D -m 0644 systemd/findsenryu4discord.target /etc/systemd/system/findsenryu4discord.target
sudo systemctl daemon-reload
sudo systemctl restart findsenryu4discord.target
```

デプロイ時に `sudo systemctl restart findsenryu-app.service` を使わないでください。`app` service だけを restart すると、Compose の `init -> migrate -> app` シーケンスを表現できません。

rootful system unit として動かすため、`systemctl --user` と `loginctl enable-linger` は使いません。

## Compose からの再生成

Linux ホスト上で `podlet` を使える場合、Compose の値を入力として `[Container]` / `[Volume]` 部分を再生成できます。

```bash
export FINDSENRYU_IMAGE="<ocir-image-uri>"

podlet --file quadlet --overwrite --name findsenryu-data podman volume create findsenryu-data

podlet --file quadlet --overwrite --name findsenryu-init podman run \
  --name findsenryu-init \
  --uidmap 0:100000:65536 \
  --gidmap 0:100000:65536 \
  -v findsenryu-data:/app/data:U \
  busybox:latest \
  sh -c "mkdir -p /app/data/backups && chown -R 65532:65532 /app/data"

podlet --file quadlet --overwrite --name findsenryu-migrate podman run \
  --name findsenryu-migrate \
  --uidmap 0:100000:65536 \
  --gidmap 0:100000:65536 \
  --entrypoint /app/migrate \
  -v findsenryu-data:/app/data \
  -v /opt/findsenryu4discord/config.toml:/app/config.toml:ro \
  -e TZ=Asia/Tokyo \
  "$FINDSENRYU_IMAGE"

podlet --file quadlet --overwrite --name findsenryu-app podman run \
  --name findsenryu-app \
  --uidmap 0:100000:65536 \
  --gidmap 0:100000:65536 \
  -p 9090:9090 \
  -v findsenryu-data:/app/data \
  -v /opt/findsenryu4discord/config.toml:/app/config.toml:ro \
  -e TZ=Asia/Tokyo \
  "$FINDSENRYU_IMAGE"
```

再生成後は、管理対象ファイルの `findsenryu-data.volume` の `VolumeName=findsenryu-data`、`findsenryu.image`、container 側の `Image=findsenryu.image`、固定 `UIDMap` / `GIDMap`、target 構成、`[Unit]` / `[Service]` の依存関係を再適用してください。`findsenryu4discord.target` は Quadlet file ではないため、`podman quadlet install --replace quadlet/` では配置されません。`systemd/findsenryu4discord.target` を systemd system unit として `/etc/systemd/system/` に配置してください。target 構成は `podman quadlet install` 後の運用単位を `findsenryu4discord.target` にし、更新時に `init` / `migrate` を再実行するために必要です。

## 参考

- Podman Quadlet overview: https://docs.podman.io/en/stable/markdown/podman-quadlet.1.html
- `podman quadlet install`: https://docs.podman.io/en/latest/markdown/podman-quadlet-install.1.html
- Quadlet file syntax: https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html
- `podman run --userns` / `--uidmap`: https://docs.podman.io/en/latest/markdown/podman-run.1.html
- systemd target units: https://www.freedesktop.org/software/systemd/man/latest/systemd.target.html
