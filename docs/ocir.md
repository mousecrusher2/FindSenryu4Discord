# OCIR デプロイ

`compose.yaml` を実行時定義の正とします。GitHub Actions は Dockerfile からイメージをビルドして Oracle Cloud Infrastructure Registry (OCIR) に push し、Compose と Quadlet はそのイメージを参照します。

## OCIR イメージ URI

イメージ URI は次の形式です。

```text
<registry-domain>/<tenancy-namespace>/<repo-name>:<tag>
```

例:

```text
<ocir-registry>/<tenancy-namespace>/<repo-name>:latest
```

テナンシ namespace は OCI のテナンシ画面に表示される Object Storage namespace です。具体的な registry hostname や image URI はリポジトリ変数や管理対象ファイルには置かず、GitHub Secrets に入れてください。

## OCI 側の準備

### 1. Registry と repository を決める

1. 使用する OCIR リージョンを決めます。
2. OCI Console で `Container Registry` を開きます。
3. 対象 compartment を選び、repository を作成します。
4. repository 名を決めます。例: `findsenryu4discord`
5. テナンシの Object Storage namespace を確認します。

GitHub Secrets に入れる値はこの時点で次のように決まります。

```text
OCIR_REGISTRY=<ocir-registry>
OCIR_IMAGE=<ocir-registry>/<tenancy-namespace>/<repo-name>:latest
```

各値の意味:

| 値 | 確認場所または決め方 |
| --- | --- |
| `<ocir-registry>` | 使用する OCIR リージョンの registry hostname |
| `<tenancy-namespace>` | Tenancy details の Object Storage namespace |
| `<repo-name>` | Container Registry に作成した repository 名 |
| `:latest` | この workflow で push する tag。別 tag にする場合は `OCIR_IMAGE` secret 側を変更します |

### 2. Push 用ユーザーと権限を用意する

push 用ユーザーを OCI IAM group に入れ、その group に repository への push 権限を付与します。

repository を事前作成する場合の例:

```text
Allow group <group-name> to manage repos in compartment <compartment-name> where ANY {request.permission = 'REPOSITORY_READ', request.permission = 'REPOSITORY_UPDATE'}
```

push 時に repository 作成も許可する場合は `REPOSITORY_CREATE` も必要です。

```text
Allow group <group-name> to manage repos in compartment <compartment-name> where ANY {request.permission = 'REPOSITORY_READ', request.permission = 'REPOSITORY_UPDATE', request.permission = 'REPOSITORY_CREATE'}
```

より単純に compartment 内の repository 管理を許可するなら次の形でも動作しますが、権限は広くなります。

```text
Allow group <group-name> to manage repos in compartment <compartment-name>
```

### 3. Auth Token を作成する

push 用ユーザーで OCI Console に入り、次の手順で Auth Token を作成します。

1. Profile menu から `User settings` を開きます。
2. `Auth Tokens` を開きます。
3. `Generate Token` を選択します。
4. 用途が分かる説明を入力して token を生成します。
5. 表示された token をすぐにコピーします。閉じると再表示できません。

管理者が別ユーザー用に作る場合は、`Identity & Security -> Users` から対象ユーザーを開いて同じ操作をします。

OCIR の login username は通常次の形式です。

```text
<tenancy-namespace>/<oci-username>
```

フェデレーションユーザーの場合は identity domain を含めます。

```text
<tenancy-namespace>/<identity-domain>/<oci-username>
```

### 4. ローカルで login を確認する

GitHub Secrets に入れる前に、手元またはデプロイ先で login できることを確認します。

```bash
export OCIR_REGISTRY="<ocir-registry>"
export OCIR_USERNAME="<tenancy-namespace>/<oci-username>"

docker login "$OCIR_REGISTRY" -u "$OCIR_USERNAME"
```

password には OCI Auth Token を入力します。

### 5. GitHub Secrets に設定する

`Settings -> Secrets and variables -> Actions -> Secrets` に次を作成します。GitHub では文字列を組み立てません。registry、image URI、username、token をそれぞれ完成した値として secret に入れます。

| Secret | 入れる値 |
| --- | --- |
| `OCIR_REGISTRY` | `docker login` の registry 引数に渡す値 |
| `OCIR_IMAGE` | `docker/build-push-action` の `tags` に渡す完全な image URI |
| `OCIR_USERNAME` | `docker login` の username |
| `OCIR_AUTH_TOKEN` | OCI Auth Token |

`.github/workflows/publish-ocir.yml` は `master` への push、`v` で始まる tag、手動実行で動作します。push する tag は `OCIR_IMAGE` secret に含めた tag です。

## Compose デプロイ

`config.toml` を用意し、イメージ URI を設定して起動します。

```bash
export OCIR_REGISTRY="<ocir-registry>"
export OCIR_USERNAME="your-tenancy-namespace/your-oci-username"
export FINDSENRYU_IMAGE="<ocir-image-uri>"

docker login "$OCIR_REGISTRY" -u "$OCIR_USERNAME"
docker compose pull
docker compose up -d
```

アプリケーションデータは Compose の named volume `findsenryu-data` に保存されます。`config.toml` は bind mount のままです。

`docker compose pull` と `docker compose up` は OCIR から image を pull するため、デプロイ先でも `docker login "$OCIR_REGISTRY" -u "$OCIR_USERNAME"` が必要です。password には OCI Auth Token を入力してください。

## 参考

- Oracle Container Registry concepts: https://docs.oracle.com/en-us/iaas/Content/Registry/Concepts/registryconcepts.htm
- Oracle Container Registry prerequisites: https://docs.oracle.com/en-us/iaas/Content/Registry/Concepts/registryprerequisites.htm
- Oracle pushing images with Docker CLI: https://docs.oracle.com/en-us/iaas/Content/Registry/Tasks/registrypushingimagesusingthedockercli.htm
- Oracle auth tokens: https://docs.oracle.com/iaas/Content/Registry/Tasks/registrygettingauthtoken.htm
- GitHub Actions secrets: https://docs.github.com/actions/reference/encrypted-secrets
