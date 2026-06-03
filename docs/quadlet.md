# Quadlet デプロイ

`compose.yaml` は OCIR image、migrate、app、external secrets の基本形を示します。Quadlet は rootful system unit として運用し、Podman secret file mount、`UserNS=auto`、`LogDriver=passthrough` を使います。

- `findsenryu.image`: OCIR image を `docker-credential-ocir` 用 authfile で pull します。
- `findsenryu-migrate.container`: 外部 PostgreSQL に対して `/app/migrate` を実行します。
- `findsenryu-app.container`: migrate 完了後に bot を起動します。
- `systemd/findsenryu4discord.target`: スタック全体の運用単位です。

named volume と `config.toml` bind mount は使いません。設定はすべて Podman secret として container 内の `/run/secrets/<secret-name>` に mount します。PostgreSQL は外部にあるものへ接続する前提で、この repository では PostgreSQL container を用意しません。

`findsenryu4discord.target` には生成 service への `Requires=` を書きません。Quadlet file と target は別に配置され、Quadlet service は `systemctl daemon-reload` 時に生成されるためです。生成後の `findsenryu-image.service` / `findsenryu-migrate.service` / `findsenryu-app.service` を enable すると、各 unit の `[Install] WantedBy=findsenryu4discord.target` により `findsenryu4discord.target.wants/` に接続されます。起動順序と失敗時の依存は各 service 側の `Requires=` / `After=` / `Before=` で表現します。

## User Namespace

app と migrate は rootful Podman で起動しますが、container は `UserNS=auto` で動かします。永続 named volume を共有しないため、固定 `UIDMap` / `GIDMap` は使いません。

`UserNS=auto` は container ごとに割り当て range が変わり得ます。host 側 file owner と共有 volume に依存する構成では問題になりますが、この構成では application data を外部 PostgreSQL に置き、config も bind mount しないため、その制約を避けています。

## Logging

`findsenryu-migrate.container` と `findsenryu-app.container` は `LogDriver=passthrough` を指定します。Podman の logging driver で stdout/stderr を再解釈せず、Podman process の stdout/stderr を systemd service の journal 入力へ渡すためです。

application log は行頭に systemd/journald が解釈する priority prefix を付けます。対応は次の通りです。

| application level | journal priority |
| --- | --- |
| `debug` | `<7>` debug |
| `info` | `<6>` info |
| `warn` | `<4>` warning |
| `error` | `<3>` err |

生成 service には `StandardOutput=journal`、`StandardError=journal`、`SyslogLevelPrefix=yes` を指定します。systemd は priority prefix を取り除いた上で journal entry の priority として保存するため、`journalctl -p warning` のような priority filter が使えます。

## Secret

Quadlet file に列挙している secret はすべて作成してください。任意設定は空 secret で構いません。空の場合は application default を使います。

| Secret | 内容 | 空の場合 |
| --- | --- | --- |
| `findsenryu-discord-token` | Discord Bot token | 不可 |
| `findsenryu-discord-playing` | Bot のプレイ中表示 | 空文字 |
| `findsenryu-discord-welcome-enabled` | welcome message の有効/無効 | `true` |
| `findsenryu-pghost` | PostgreSQL host | 不可 |
| `findsenryu-pgdatabase` | PostgreSQL database | 不可 |
| `findsenryu-pguser` | PostgreSQL user | 不可 |
| `findsenryu-pgpassword` | PostgreSQL password | 不可 |
| `findsenryu-pgsslmode` | PostgreSQL sslmode。Neon では `verify-full` 推奨 | 不可 |
| `findsenryu-log-level` | `debug` / `info` / `warn` / `error` | `info` |
| `findsenryu-log-format` | `text` / `json` | `text` |
| `findsenryu-admin-owner-ids` | owner Discord ID。複数は comma 区切り | 空 |
| `findsenryu-admin-guild-id` | 管理コマンド登録先 guild ID | 空 |
| `findsenryu-admin-log-channel-id` | 参加/離脱通知 channel ID | 空 |
| `findsenryu-admin-report-channel-id` | 日次 report channel ID | 空 |
| `findsenryu-admin-contact-channel-id` | `/contact` 通知 channel ID | 空 |
| `findsenryu-server-enabled` | health/metrics server の有効/無効 | `true` |
| `findsenryu-server-port` | health/metrics server port | `9090` |

作成例:

```bash
printf '%s' '<discord-bot-token>' | sudo podman secret create --replace findsenryu-discord-token -
printf '\n' | sudo podman secret create --replace findsenryu-discord-playing -
printf '%s' 'true' | sudo podman secret create --replace findsenryu-discord-welcome-enabled -
printf '%s' '<postgres-host>' | sudo podman secret create --replace findsenryu-pghost -
printf '%s' '<postgres-database>' | sudo podman secret create --replace findsenryu-pgdatabase -
printf '%s' '<postgres-user>' | sudo podman secret create --replace findsenryu-pguser -
printf '%s' '<postgres-password>' | sudo podman secret create --replace findsenryu-pgpassword -
printf '%s' 'verify-full' | sudo podman secret create --replace findsenryu-pgsslmode -
printf '%s' 'info' | sudo podman secret create --replace findsenryu-log-level -
printf '%s' 'text' | sudo podman secret create --replace findsenryu-log-format -
printf '\n' | sudo podman secret create --replace findsenryu-admin-owner-ids -
printf '\n' | sudo podman secret create --replace findsenryu-admin-guild-id -
printf '\n' | sudo podman secret create --replace findsenryu-admin-log-channel-id -
printf '\n' | sudo podman secret create --replace findsenryu-admin-report-channel-id -
printf '\n' | sudo podman secret create --replace findsenryu-admin-contact-channel-id -
printf '%s' 'true' | sudo podman secret create --replace findsenryu-server-enabled -
printf '%s' '9090' | sudo podman secret create --replace findsenryu-server-port -
```

## インストール

OCIR image は `quadlet/findsenryu.image` に固定しています。

```text
kix.ocir.io/axkvg5nxhc7t/senryu:latest
```

OCIR の pull 認証は `docker-credential-ocir` を使います。credential helper の config 値は `ocir` です。rootful systemd unit から pull するため、helper と OCI CLI は root から実行できる状態にしてください。

```bash
sudo sh -c 'command -v docker-credential-ocir'
sudo oci iam region list --auth instance_principal

sudo install -d -m 0700 /etc/containers/auth
sudo sh -c 'cat > /etc/containers/auth/findsenryu4discord.json' <<'EOF'
{
  "credHelpers": {
    "kix.ocir.io": "ocir"
  }
}
EOF
sudo chown root:root /etc/containers/auth/findsenryu4discord.json
sudo chmod 600 /etc/containers/auth/findsenryu4discord.json
```

`docker-credential-ocir` は OCI CLI を使って OCIR token を取得します。OCI instance principal で認証する前提では `podman login` と静的な OCI Auth Token は使いません。deploy host の instance を Dynamic Group に入れ、OCIR repository を pull できる policy を付与してください。

Quadlet と target を install します。`findsenryu4discord.target` は Quadlet file ではないため、`podman quadlet install --replace quadlet/` では配置されません。

```bash
sudo podman quadlet install --replace quadlet/
sudo install -D -m 0644 systemd/findsenryu4discord.target /etc/systemd/system/findsenryu4discord.target
sudo systemctl daemon-reload
sudo systemctl enable findsenryu-image.service findsenryu-migrate.service findsenryu-app.service
sudo systemctl enable --now findsenryu4discord.target
```

`sudo podman quadlet install --replace quadlet/` の出力先は rootful Quadlet search path である `/etc/containers/systemd/` であることを確認してください。`findsenryu.image` は `AuthFile=/etc/containers/auth/findsenryu4discord.json`、`Image=kix.ocir.io/axkvg5nxhc7t/senryu:latest`、`Policy=always` を指定しています。この authfile は `kix.ocir.io` に対して credential helper `ocir` を使う設定です。`findsenryu-migrate.container` と `findsenryu-app.container` は `Image=findsenryu.image` を参照するため、起動時の pull は `findsenryu-image.service` 経由で実行されます。

ログ確認:

```bash
sudo journalctl -u findsenryu-app.service -f
sudo journalctl -u findsenryu-app.service -p warning
```

イメージや Quadlet file を更新した場合は target を restart します。これにより `migrate` が実行されてから `app` が起動します。

```bash
sudo podman quadlet install --replace quadlet/
sudo install -D -m 0644 systemd/findsenryu4discord.target /etc/systemd/system/findsenryu4discord.target
sudo systemctl daemon-reload
sudo systemctl enable findsenryu-image.service findsenryu-migrate.service findsenryu-app.service
sudo systemctl restart findsenryu4discord.target
```

デプロイ時に `sudo systemctl restart findsenryu-app.service` を使わないでください。`app` service だけを restart すると、`migrate -> app` シーケンスを表現できません。

rootful system unit として動かすため、`systemctl --user` と `loginctl enable-linger` は使いません。

## Podlet

`[Container]` の基本部分は `podlet 0.3.2` の `podlet podman run` で確認できます。

```bash
image="kix.ocir.io/axkvg5nxhc7t/senryu:latest"

podlet --file quadlet --overwrite --name findsenryu-migrate podman run \
  --name findsenryu-migrate \
  --userns auto \
  --log-driver passthrough \
  --secret findsenryu-discord-token \
  --secret findsenryu-discord-playing \
  --secret findsenryu-discord-welcome-enabled \
  --secret findsenryu-pghost \
  --secret findsenryu-pgdatabase \
  --secret findsenryu-pguser \
  --secret findsenryu-pgpassword \
  --secret findsenryu-pgsslmode \
  --secret findsenryu-log-level \
  --secret findsenryu-log-format \
  --secret findsenryu-admin-owner-ids \
  --secret findsenryu-admin-guild-id \
  --secret findsenryu-admin-log-channel-id \
  --secret findsenryu-admin-report-channel-id \
  --secret findsenryu-admin-contact-channel-id \
  --secret findsenryu-server-enabled \
  --secret findsenryu-server-port \
  --entrypoint /app/migrate \
  "$image"

podlet --file quadlet --overwrite --name findsenryu-app podman run \
  --name findsenryu-app \
  --userns auto \
  --log-driver passthrough \
  --secret findsenryu-discord-token \
  --secret findsenryu-discord-playing \
  --secret findsenryu-discord-welcome-enabled \
  --secret findsenryu-pghost \
  --secret findsenryu-pgdatabase \
  --secret findsenryu-pguser \
  --secret findsenryu-pgpassword \
  --secret findsenryu-pgsslmode \
  --secret findsenryu-log-level \
  --secret findsenryu-log-format \
  --secret findsenryu-admin-owner-ids \
  --secret findsenryu-admin-guild-id \
  --secret findsenryu-admin-log-channel-id \
  --secret findsenryu-admin-report-channel-id \
  --secret findsenryu-admin-contact-channel-id \
  --secret findsenryu-server-enabled \
  --secret findsenryu-server-port \
  -p 9090:9090 \
  "$image"
```

再生成後は、管理対象ファイルの `findsenryu.image`、container 側の `Image=findsenryu.image`、target 構成、`[Unit]` / `[Service]` の依存関係、`[Install] WantedBy=findsenryu4discord.target` を再適用してください。`findsenryu4discord.target` は Quadlet file ではないため、`podman quadlet install --replace quadlet/` では配置されません。`systemd/findsenryu4discord.target` を systemd system unit として `/etc/systemd/system/` に配置してください。

## 参考

- Podman Quadlet overview: https://docs.podman.io/en/stable/markdown/podman-quadlet.1.html
- `podman quadlet install`: https://docs.podman.io/en/latest/markdown/podman-quadlet-install.1.html
- Quadlet file syntax: https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html
- Podman secrets: https://docs.podman.io/en/latest/markdown/podman-secret-create.1.html
- `podman run --secret` / `--userns` / `--log-driver`: https://docs.podman.io/en/latest/markdown/podman-run.1.html
- containers auth.json credential helpers: https://manpages.ubuntu.com/manpages/noble/man5/containers-auth.json.5.html
- OCIR Docker credential helper: https://docs.oracle.com/en/learn/cred-helper/index.html
- systemd target units: https://www.freedesktop.org/software/systemd/man/latest/systemd.target.html
- systemd stdout/stderr priority prefix: https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html
