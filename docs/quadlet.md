# Quadlet デプロイ

Quadlet は OCIR image、migrate、app、external secrets の構成を rootful system unit として運用します。Podman secret file mount、`UserNS=auto`、`LogDriver=passthrough` を使います。

- `findsenryu.image`: OCIR image を `docker-credential-ocir` 用 authfile で pull します。
- `findsenryu-migrate.container`: 外部 PostgreSQL に対して `/app/migrate` を実行します。
- `findsenryu-app.container`: migrate 完了後に bot を起動します。
- `systemd/findsenryu4discord.target`: スタック全体の運用単位です。

named volume と `config.toml` bind mount は使いません。設定はすべて Podman secret として container 内の `/run/secrets/<secret-name>` に mount します。PostgreSQL は外部にあるものへ接続する前提で、この repository では PostgreSQL container を用意しません。

Quadlet generator が生成する service は transient unit であるため `systemctl enable` は使えません。代わりに Quadlet file の `[Install] WantedBy=findsenryu4discord.target` を generator が生成時に処理し、`systemctl enable` 相当の `.wants/` symlink を自動的に作成します。そのため `findsenryu4discord.target` 側に `Wants=` を書く必要はありません。起動順序と失敗時の依存は各 service 側の `Requires=` / `After=` / `Before=` で表現します。また、`findsenryu-migrate` (oneshot型) には `Restart=on-failure` を設定しており、DB 起動の遅延などで失敗した場合も、systemd が成功するまで自動で再試行します。

## User Namespace

app と migrate は rootful Podman で起動しますが、container は `UserNS=auto` で動かします。永続 named volume を共有しないため、固定 `UIDMap` / `GIDMap` は使いません。

`UserNS=auto` は container ごとに割り当て range が変わり得ます。host 側 file owner と共有 volume に依存する構成では問題になりますが、この構成では application data を外部 PostgreSQL に置き、config も bind mount しないため、その制約を避けています。

### subuid / subgid の範囲拡張

複数コンテナで `UserNS=auto` を利用する場合、各コンテナに割り当てるための十分な UID/GID の範囲が `/etc/subuid` および `/etc/subgid` に定義されている必要があります。

デフォルトの割当数（`containers:100000:65536` など）では、1つのコンテナが範囲全体（65,536個）を消費してしまい、他の `UserNS=auto` を使うコンテナが `not enough unused uids` エラーで起動できなくなります。

以下のように `/etc/subuid` と `/etc/subgid` の割当数を増やすことで、複数の `UserNS=auto` コンテナを同時に起動できるようになります。

```bash
# 既存の割り当てを 1000000 (100万) に拡張する例
sudo sed -i 's/containers:100000:65536/containers:100000:1000000/' /etc/subuid
sudo sed -i 's/containers:100000:65536/containers:100000:1000000/' /etc/subgid
```

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

## 定期監視

この構成では Podman の定期的な死活判定も application 側の監視用 HTTP server も使いません。PostgreSQL は Neon の scale to zero を利用する前提であり、定期的に DB ping を実行すると compute を起こしたり、suspend 後の接続再確立を異常と誤判定したりする可能性があるためです。

プロセスが終了した場合の復旧は systemd の `Restart=always` で扱います。DB の状態は実際の application 処理で必要になった時点で確認され、Neon が suspend していた場合は通常の DB 接続処理に任せます。

## Secret

Quadlet file に列挙している secret はすべて作成してください。任意設定は空 secret で構いません。空の場合は application default を使います。

`findsenryu-app.container` は Discord、PostgreSQL、log、admin の secret を読みます。`findsenryu-migrate.container` は schema migration に必要な PostgreSQL と log の secret だけを読みます。

| Secret | 内容 | 空の場合 |
| --- | --- | --- |
| `findsenryu-discord-token` | Discord Bot token | 不可 |
| `findsenryu-discord-playing` | Bot のプレイ中表示 | 空文字 |
| `findsenryu-pghost` | PostgreSQL host | 不可 |
| `findsenryu-pgdatabase` | PostgreSQL database | 不可 |
| `findsenryu-pguser` | PostgreSQL user | 不可 |
| `findsenryu-pgpassword` | PostgreSQL password | 不可 |
| `findsenryu-pgsslmode` | PostgreSQL sslmode。Neon では `verify-full` 推奨 | 不可 |
| `findsenryu-log-level` | `debug` / `info` / `warn` / `error` | `info` |
| `findsenryu-log-format` | `text` / `json` | `text` |
| `findsenryu-admin-owner-ids` | owner Discord ID。複数は comma 区切り | 空 |
| `findsenryu-admin-guild-id` | 管理コマンド登録先 guild ID | 空 |

作成例:

```bash
printf '%s' '<discord-bot-token>' | sudo podman secret create --replace findsenryu-discord-token -
printf '\n' | sudo podman secret create --replace findsenryu-discord-playing -
printf '%s' '<postgres-host>' | sudo podman secret create --replace findsenryu-pghost -
printf '%s' '<postgres-database>' | sudo podman secret create --replace findsenryu-pgdatabase -
printf '%s' '<postgres-user>' | sudo podman secret create --replace findsenryu-pguser -
printf '%s' '<postgres-password>' | sudo podman secret create --replace findsenryu-pgpassword -
printf '%s' 'verify-full' | sudo podman secret create --replace findsenryu-pgsslmode -
printf '%s' 'info' | sudo podman secret create --replace findsenryu-log-level -
printf '%s' 'text' | sudo podman secret create --replace findsenryu-log-format -
printf '\n' | sudo podman secret create --replace findsenryu-admin-owner-ids -
printf '\n' | sudo podman secret create --replace findsenryu-admin-guild-id -
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

Quadlet と target を install します。repository を clone する必要はありません。

`findsenryu4discord.target` は Quadlet file ではないため、`podman quadlet install` では配置できません。`curl` で GitHub から直接取得して `/etc/systemd/system/` に配置します。

新規インストール時は衝突を検出するために、既存のファイルを自動削除せずに `podman quadlet install` を実行します（すでに同一名の定義ファイルが存在する場合はエラーを返します）。

各 Quadlet file の `[Install] WantedBy=findsenryu4discord.target` を generator が処理し、`.wants/` symlink を自動作成するため `systemctl enable` は不要です。

```bash
repo=https://github.com/mousecrusher2/FindSenryu4Discord

# 1. target の配置
sudo curl -fsSL -o /etc/systemd/system/findsenryu4discord.target \
  "$repo/raw/refs/heads/master/systemd/findsenryu4discord.target"
sudo chmod 0644 /etc/systemd/system/findsenryu4discord.target

# 2. Quadlet ファイルのインストール（既存ファイルがある場合は衝突を検出してエラーになります）
sudo podman quadlet install \
  "$repo/raw/refs/heads/master/quadlet/findsenryu.image" \
  "$repo/raw/refs/heads/master/quadlet/findsenryu-migrate.container" \
  "$repo/raw/refs/heads/master/quadlet/findsenryu-app.container"

# 3. スタック全体の有効化と起動
sudo systemctl enable --now findsenryu4discord.target
```

`findsenryu.image` は `AuthFile=/etc/containers/auth/findsenryu4discord.json`、`Image=kix.ocir.io/axkvg5nxhc7t/senryu:latest`、`Policy=always` を指定しています。この authfile は `kix.ocir.io` に対して credential helper `ocir` を使う設定です。`findsenryu-migrate.container` と `findsenryu-app.container` は `Image=findsenryu.image` を参照するため、起動時の pull は `findsenryu-image.service` 経由で実行されます。

ログ確認:

```bash
sudo journalctl -u findsenryu-app.service -f
sudo journalctl -u findsenryu-app.service -p warning
```

イメージや Quadlet file を更新した場合は、同様に古いファイルを削除してから再インストールし、target を restart します。これにより `migrate` が実行されてから `app` が起動します。

```bash
repo=https://github.com/mousecrusher2/FindSenryu4Discord

# 1. target の再取得
sudo curl -fsSL -o /etc/systemd/system/findsenryu4discord.target \
  "$repo/raw/refs/heads/master/systemd/findsenryu4discord.target"
sudo chmod 0644 /etc/systemd/system/findsenryu4discord.target

# 2. 古い Quadlet 定義ファイルの削除
sudo rm -f /etc/containers/systemd/findsenryu.image \
           /etc/containers/systemd/findsenryu-migrate.container \
           /etc/containers/systemd/findsenryu-app.container

# 3. Quadlet ファイルの再インストール
sudo podman quadlet install \
  "$repo/raw/refs/heads/master/quadlet/findsenryu.image" \
  "$repo/raw/refs/heads/master/quadlet/findsenryu-migrate.container" \
  "$repo/raw/refs/heads/master/quadlet/findsenryu-app.container"

# 4. 反映と再起動
sudo systemctl restart findsenryu4discord.target
```

デプロイ時に `sudo systemctl restart findsenryu-app.service` を使わないでください。`app` service だけを restart すると、`migrate -> app` シーケンスを表現できません。

rootful system unit として動かすため、`systemctl --user` と `loginctl enable-linger` は使いません。

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
