# FindSenryu4Discord

<p align="center">
  <img src="./.github/img/haiku.png" width="200" /><br />
  Discordで川柳を検出します
</p>

## このリポジトリについて

このリポジトリは [u16-io/FindSenryu4Discord](https://github.com/u16-io/FindSenryu4Discord) の fork です。

## Commands

### メッセージコマンド

```
詠め
```

> 今までにギルド内で詠まれた句をもとに、新しい川柳を生成します。

```
詠むな
```

> 理不尽な要求なので、最後に詠んだ人とその内容を晒しあげます。

### スラッシュコマンド

```
/mute
```

> このチャンネルでの川柳検出をミュートします。親チャンネルをミュートすると、その中のスレッドでも検出がスキップされます。

```
/unmute
```

> このチャンネルでの川柳検出のミュートを解除します。

```
/rank
```

> ギルド内で詠んだ回数が多い人のランキングを表示します。

## Self-hosting

OCIR/Quadlet で運用する場合は、先に次の手順を参照してください。

- [OCIR Deployment](docs/ocir.md)
- [Quadlet Deployment](docs/quadlet.md)

### 設定

設定は Podman secret file で渡します。Quadlet が配置環境の Podman secret 名をアプリケーション固定の `/run/secrets/<target>` へ対応付けます。データベースは PostgreSQL 専用です。SQLite は production runtime では使いません。

必須:

```text
findsenryu-discord-token
findsenryu-pghost
findsenryu-pgdatabase
findsenryu-pguser
findsenryu-pgpassword
findsenryu-pgsslmode
```

主な任意設定:

```text
findsenryu-log-level
```

Neon に接続する場合、`findsenryu-pgsslmode` は `verify-full` を設定してください。

### 機能

- **川柳検出** — メッセージから5-7-5の音節パターンを自動検出・記録。テキストチャンネル、スレッド、フォーラム投稿、ボイスチャンネル、ステージチャンネルに対応
- **チャンネルミュート** — チャンネル単位で検出を無効化。親チャンネルをミュートすると、その中のスレッドでも検出がスキップされます
- **自動ミュート** — Bot権限不足（Missing Access / Missing Permissions / メッセージ履歴読み取り不可）で返信に失敗した場合、該当チャンネルを自動的にミュートします（`/unmute` で解除可能）

## License

This project is licensed under the [MIT License](LICENSE).
