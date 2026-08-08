---
name: install
description: クローンしたこの revu リポジトリから、このマシンに revu 一式（CLI バイナリ + Claude Code プラグイン + Codex CLI プラグイン）をインストール・更新する。「インストールして」「セットアップして」「環境を作って」「install」などと言われたら使う。git pull 後の更新にも使える。
---

# install

クローン直後（または `git pull` 直後）のこのリポジトリから、revu 一式をこのマシンにセットアップする。

インストール対象は 3 つ。それぞれ独立しており、途中で失敗しても他は続行する:

| 対象 | 方法 | 更新時 |
|---|---|---|
| CLI バイナリ | `make install` | 再実行が必要 |
| Claude Code プラグイン (`/revu:pr`, `/revu:edit`) | `make install-skills`（symlink） | 不要（symlink が repo を指す） |
| Codex CLI プラグイン (`$revu:pr`, `$revu:edit`) | `scripts/install-codex.sh` | 再実行が必要（cache にコピーされるため） |

## 手順

### 1. 前提確認

```bash
git rev-parse --show-toplevel
go version
```

- cwd がこのリポジトリ内であること（違えばリポジトリルートで実行するよう案内して終了）
- Go 1.23 以上であること。無ければインストール案内を出して終了

### 2. CLI バイナリのインストール

`~/.local/bin` が `$PATH` に含まれるかを確認し、含まれるなら sudo 不要の以下を実行:

```bash
make install PREFIX=$HOME/.local
```

`~/.local/bin` が `$PATH` に無い場合は、`/usr/local/bin` への配置に sudo が必要なことを伝え、ユーザーに `! sudo make install` の実行を案内する（Claude からは sudo を実行しない）。

インストール後に検証:

```bash
revu version
```

`which revu` が古いパスを指している場合はその旨を報告する。

### 3. Claude Code プラグインのインストール

```bash
make install-skills
```

- `~/.claude/skills/revu` → `<repo>/plugin` の symlink が張られる
- `skip: ... already exists (not a symlink)` が出たら、既存の実体ディレクトリがあるということ。中身を確認してユーザーに置き換えてよいか確認する
- 旧構成の `~/.claude/skills/review-pr`（standalone 版の symlink）が残っていたら、機能重複するので削除を提案する
- 反映は Claude Code の**次セッションから**であることを伝える

### 4. Codex CLI プラグインのインストール（codex がある場合のみ）

`codex` が `$PATH` にあるか確認し、あれば:

```bash
scripts/install-codex.sh
```

- リポジトリがマーケットプレース `revu` として登録され、プラグインが `~/.codex/plugins/cache/` にコピーされる
- 既に登録済みで再インストールになる場合、エラーが出たら `scripts/install-codex.sh --uninstall` してから再実行する
- 反映には Codex の再起動が必要なことを伝える

`codex` が無ければこのステップはスキップし、その旨だけ報告する。

### 5. permission 設定の案内（初回のみ）

`~/.claude/settings.json` に revu 用の permission が無いと、`revu review` の内部で起動される `claude --print` が権限プロンプトを出せずに失敗する。設定の有無を確認し、無ければ README の「`revu review` で claude を使うときの permission」節にある以下の追加を提案する（勝手に書き換えず、ユーザーに確認を取る）:

```json
{
  "permissions": {
    "allow": [
      "Bash(revu *)",
      "Read",
      "Write(/home/<user>/.revu/**)"
    ]
  }
}
```

### 6. 結果報告

各対象について 成功 / スキップ / 失敗 を表で報告する。あわせて次を伝える:

- Claude Code は次セッションから `/revu:pr <PR_NUMBER>` と `/revu:edit` が使える
- Codex は再起動後に `$revu:pr <PR_NUMBER>` が使える
- 以後の更新は `git pull` してから、この skill をもう一度実行すればよい（バイナリと Codex cache だけ入れ直され、Claude 側 symlink はそのまま）
