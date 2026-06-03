# OCIR デプロイ

GitHub Actions は Dockerfile からイメージをビルドして Oracle Cloud Infrastructure Registry (OCIR) に push します。Compose と Quadlet はそのイメージを参照します。deploy host での OCIR pull 認証は `docker-credential-ocir` と credential helper 名 `ocir` を使う前提です。

## OCIR イメージ URI

この repository では OCIR image URI を固定しています。

```text
kix.ocir.io/axkvg5nxhc7t/senryu:latest
```

構成要素:

| 値 | 内容 |
| --- | --- |
| `kix.ocir.io` | Osaka region の OCIR registry hostname |
| `axkvg5nxhc7t` | Tenancy Object Storage namespace |
| `senryu` | OCIR repository name |
| `latest` | push/pull する tag |

## OCI 側の準備

### 1. Registry と repository を決める

1. 使用する OCIR リージョンを決めます。
2. OCI Console で `Container Registry` を開きます。
3. 対象 compartment を選び、repository を作成します。
4. repository 名は `senryu` です。
5. テナンシの Object Storage namespace を確認します。

この repository で使う値:

```text
OCIR_REGISTRY=kix.ocir.io
OCIR_IMAGE=kix.ocir.io/axkvg5nxhc7t/senryu:latest
```

各値の意味:

| 値 | 確認場所または決め方 |
| --- | --- |
| `kix.ocir.io` | 使用する OCIR registry hostname |
| `axkvg5nxhc7t` | Tenancy details の Object Storage namespace |
| `senryu` | Container Registry に作成する repository 名 |
| `:latest` | この workflow で push する tag |

### 2. Push と deploy の権限を用意する

push 用ユーザーを OCI IAM group に入れ、その group に repository への push 権限を付与します。

repository を事前作成する場合の例:

```text
Allow group <group-name> to manage repos in compartment <compartment-name> where ANY {request.permission = 'REPOSITORY_INSPECT', request.permission = 'REPOSITORY_READ', request.permission = 'REPOSITORY_UPDATE'}
```

`REPOSITORY_INSPECT` は Actions で古い image を列挙するために使います。`REPOSITORY_UPDATE` は push と image delete の両方に使います。

push 時に repository 作成も許可する場合は `REPOSITORY_CREATE` も必要です。

```text
Allow group <group-name> to manage repos in compartment <compartment-name> where ANY {request.permission = 'REPOSITORY_INSPECT', request.permission = 'REPOSITORY_READ', request.permission = 'REPOSITORY_UPDATE', request.permission = 'REPOSITORY_CREATE'}
```

より単純に compartment 内の repository 管理を許可するなら次の形でも動作しますが、権限は広くなります。

```text
Allow group <group-name> to manage repos in compartment <compartment-name>
```

deploy host は `docker-credential-ocir` と OCI instance principal で OCIR へ認証する前提です。deploy host の instance を Dynamic Group に入れ、その Dynamic Group に pull 権限を付与します。

```text
Allow dynamic-group <dynamic-group-name> to read repos in compartment <compartment-name>
```

deploy host から push も行う場合は `manage repos` が必要です。

```text
Allow dynamic-group <dynamic-group-name> to manage repos in compartment <compartment-name>
```

### 3. GitHub Actions 用 Auth Token を作成する

GitHub-hosted runner で `.github/workflows/publish-ocir.yml` を使う場合は、push 用ユーザーで OCI Console に入り、次の手順で Auth Token を作成します。

1. Profile menu から `User settings` を開きます。
2. `Auth Tokens` を開きます。
3. `Generate Token` を選択します。
4. 用途が分かる説明を入力して token を生成します。
5. 表示された token をすぐにコピーします。閉じると再表示できません。

管理者が別ユーザー用に作る場合は、`Identity & Security -> Users` から対象ユーザーを開いて同じ操作をします。

OCIR の login username は workflow 側で `axkvg5nxhc7t/` を前置します。`OCIR_USERNAME` secret には slash の後ろだけを入れます。通常ユーザーなら次の形式です。

```text
<oci-username>
```

フェデレーションユーザーの場合は identity domain 以降を入れます。

```text
<identity-domain>/<oci-username>
```

### 4. Deploy host で credential helper を確認する

deploy host には `docker-credential-ocir` と OCI CLI を入れます。credential helper の config 値は `ocir` です。

```bash
command -v docker-credential-ocir
oci iam region list --auth instance_principal

mkdir -p "$HOME/.docker"
cat > "$HOME/.docker/config.json" <<'EOF'
{
  "credHelpers": {
    "kix.ocir.io": "ocir"
  }
}
EOF

docker pull "kix.ocir.io/axkvg5nxhc7t/senryu:latest"
```

`docker-credential-ocir` は OCI CLI を使って OCIR token を取得します。instance principal で認証する前提では、deploy host で `docker login` や静的な OCI Auth Token を使いません。

### 5. GitHub Secrets に設定する

`Settings -> Secrets and variables -> Actions -> Secrets` に次を作成します。registry と image URI は workflow に固定しているため secret にしません。

OCIR へ push する OCI user と cleanup に使う OCI user は同じでかまいません。ただし、`docker/login-action` の username/Auth Token は Docker Registry 用の認証情報であり、OCI CLI の API 署名には使えません。同じ publish user に OCI API key を登録し、その user OCID、tenancy OCID、fingerprint、private key を OCI CLI 用 secret として渡します。

| Secret | 入れる値 |
| --- | --- |
| `OCIR_USERNAME` | OCIR login username の namespace より後ろ。通常は `<oci-username>`、フェデレーションユーザーは `<identity-domain>/<oci-username>` |
| `OCIR_AUTH_TOKEN` | OCI Auth Token |
| `OCI_CLI_USER` | OCI API key を登録した user OCID |
| `OCI_CLI_TENANCY` | tenancy OCID |
| `OCI_CLI_FINGERPRINT` | OCI API key fingerprint |
| `OCI_CLI_KEY_CONTENT` | OCI API private key PEM の内容 |
| `OCI_CLI_REGION` | OCIR region。大阪なら `ap-osaka-1` |

`.github/workflows/publish-ocir.yml` は `master` への push と手動実行で動作します。push する tag は `kix.ocir.io/axkvg5nxhc7t/senryu:latest` です。`senryu` repository は root compartment に置く前提です。push 後に OCI CLI で root compartment の AVAILABLE image を作成時刻の降順で取得し、直近 5 件だけを残して古い image を削除します。

## Compose デプロイ

外部 PostgreSQL の接続情報と Discord Bot token は secret として渡します。PostgreSQL container はこの Compose では用意しません。

```bash
printf '%s' '<discord-bot-token>' | podman secret create --replace findsenryu-discord-token -
printf '\n' | podman secret create --replace findsenryu-discord-playing -
printf '%s' 'true' | podman secret create --replace findsenryu-discord-welcome-enabled -
printf '%s' '<postgres-host>' | podman secret create --replace findsenryu-pghost -
printf '%s' '<postgres-database>' | podman secret create --replace findsenryu-pgdatabase -
printf '%s' '<postgres-user>' | podman secret create --replace findsenryu-pguser -
printf '%s' '<postgres-password>' | podman secret create --replace findsenryu-pgpassword -
printf '%s' 'verify-full' | podman secret create --replace findsenryu-pgsslmode -
printf '%s' 'info' | podman secret create --replace findsenryu-log-level -
printf '%s' 'text' | podman secret create --replace findsenryu-log-format -
printf '\n' | podman secret create --replace findsenryu-admin-owner-ids -
printf '\n' | podman secret create --replace findsenryu-admin-guild-id -
printf '\n' | podman secret create --replace findsenryu-admin-log-channel-id -
printf '\n' | podman secret create --replace findsenryu-admin-report-channel-id -
printf '\n' | podman secret create --replace findsenryu-admin-contact-channel-id -
printf '%s' 'true' | podman secret create --replace findsenryu-server-enabled -
printf '%s' '9090' | podman secret create --replace findsenryu-server-port -

command -v docker-credential-ocir
oci iam region list --auth instance_principal
mkdir -p "$HOME/.docker"
cat > "$HOME/.docker/config.json" <<'EOF'
{
  "credHelpers": {
    "kix.ocir.io": "ocir"
  }
}
EOF

docker compose pull
docker compose up -d
```

`compose.yaml` は external secret を宣言します。実行する Compose 実装が external secret を扱えない場合は、Quadlet 手順を使ってください。アプリケーションデータは外部 PostgreSQL に保存されます。Compose は named volume と `config.toml` bind mount を使いません。

`compose.yaml` の app service は `/app/healthcheck` で `/health` を確認します。distroless image に shell や curl は入れません。`findsenryu-server-enabled` は `true` のままにしてください。

`docker compose pull` と `docker compose up` は OCIR から image を pull するため、デプロイ先の Docker config には `kix.ocir.io` の `credHelpers` として `ocir` を設定してください。helper binary は `docker-credential-ocir` という名前で `PATH` から実行できる必要があります。

## 参考

- Oracle Container Registry concepts: https://docs.oracle.com/en-us/iaas/Content/Registry/Concepts/registryconcepts.htm
- Oracle Container Registry prerequisites: https://docs.oracle.com/en-us/iaas/Content/Registry/Concepts/registryprerequisites.htm
- Oracle pushing images with Docker CLI: https://docs.oracle.com/en-us/iaas/Content/Registry/Tasks/registrypushingimagesusingthedockercli.htm
- Oracle auth tokens: https://docs.oracle.com/iaas/Content/Registry/Tasks/registrygettingauthtoken.htm
- OCIR Docker credential helper: https://docs.oracle.com/en/learn/cred-helper/index.html
- Docker credential helpers: https://docs.docker.com/reference/cli/docker/login/#credential-stores
- GitHub Actions secrets: https://docs.github.com/actions/reference/encrypted-secrets
