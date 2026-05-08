# revu

Claude Code が生成した PR レビューを TUI で確認・編集し、GitHub に投稿するためのツール。

レビュー本文は YAML + Markdown のハイブリッド構造でローカルに保存され、TUI で取捨選択・推敲してから明示的なコマンドで GitHub に投稿します。

## ステータス

開発中（MVP）。Phase 1〜5 完了相当。

## 全体ワークフロー

```
[1] Claude Code で /review-pr <PR_NUMBER> を実行
        または
    revu review [PR_NUMBER]   ← cwd リポジトリで自分にレビュー依頼が来ている PR
                                を選び、内部で claude CLI を起動してレビュー生成
        ↓
[2] ~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/ に review.yml + summary.md + comments/*.md が出力される
        ↓
[3] revu open で TUI を起動（revu review 経由なら自動で開く）
        ↓
[4] 一覧 / 詳細 / サマリ画面でコメントを accept / reject / edit
        ↓
[5] revu submit (または TUI 内 :submit) で GitHub に投稿
```

## ビルド

```bash
make build
./bin/revu version
```

`go install ./cmd/revu` でも `$GOPATH/bin` に配置できます。

## インストール

ソースから build したバイナリを `/usr/local/bin` に置く場合:

```bash
sudo make install
```

sudo を避けたいときは `PREFIX` で配置先を変更:

```bash
make install PREFIX=$HOME/.local       # → ~/.local/bin/revu
```

アンインストールは:

```bash
sudo make uninstall                     # /usr/local/bin から
make uninstall PREFIX=$HOME/.local      # 別 PREFIX で入れた場合
```

リリース版の prebuilt バイナリを使いたい場合は `install.sh` を参照してください。

## 必要なもの

- Go 1.23 以上
- `gh` CLI（`revu submit` および `review-pr` skill で使用、`gh auth login` 済みであること）
- 投稿対象 PR のローカル clone（`revu open` を実行する場所）
- Claude Code（レビュー生成に `review-pr` skill を使う場合）

## Claude Code skill のインストール

レビュー生成は Claude Code の `review-pr` skill が担当します。本リポジトリの `skills/review-pr/` を `~/.claude/skills/` にシンボリックリンクして使います。

```bash
ln -s "$PWD/skills/review-pr" "$HOME/.claude/skills/review-pr"
```

Claude Code に `/review-pr <PR_NUMBER>` と入力すると skill が起動し、`~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/` 配下にレビューを書き出します（SHA は PR の `head_sha` 先頭 7 文字）。

```
/review-pr 123
/review-pr 123 --focus security,perf
```

skill が完了したら `revu open` で開けます。

### `revu review` で `--print` モードを使うときの permission

`revu review` は内部で `claude --print` を起動するので、未許可ツールが呼ばれるとプロンプトなしで失敗します。skill が使う操作はすべて `revu` のサブコマンドにラップ済みなので、`~/.claude/settings.json`（ユーザー全体設定）に以下を入れれば足ります:

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

skill 内で使われる revu サブコマンドは:

| サブコマンド | 用途 |
|---|---|
| `revu pr prepare <N>` | `gh pr view` + `mkdir -p` を 1 回で実行し、メタ情報と出力先を JSON で返す |
| `revu pr diff <N>` | `gh pr diff` のラッパー |
| `revu pr list-mine` | 自分にレビュー依頼が来ている open PR の一覧（`revu review` の picker からも利用） |
| `revu severities --json` | 設定されている severity セットを返す |
| `revu now` | ISO 8601 タイムスタンプ |
| `revu validate <dir>` | 生成物のスキーマ整合性チェック |

## テンプレートのカスタマイズ

`review-pr` skill が生成するサマリとインラインコメントの構造はテンプレートで決まっています。デフォルトは `~/.claude/skills/review-pr/templates/` 配下:

- `summary.md.tmpl` — PR 全体サマリ
- `inline-comment.md.tmpl` — 各インラインコメント

**ユーザー上書き** は `~/.config/revu/templates/` に同名ファイルを置くと適用されます (`$REVU_TEMPLATES` 環境変数で別ディレクトリ指定可能)。

```bash
mkdir -p ~/.config/revu/templates
cp ~/.claude/skills/review-pr/templates/summary.md.tmpl ~/.config/revu/templates/
# ↑ お手本としてコピーしてから編集する
```

テンプレートはあくまで「お手本」で、Claude が構造ガイドとして参照するだけです。固定の文字列置換ではありません。

## コマンド一覧

| コマンド | 用途 |
|---|---|
| `revu version` | バージョン表示 |
| `revu review [PR_NUMBER]` | 自分にレビュー依頼が来ている PR を選び、`claude` CLI で `/review-pr` を実行して生成された結果を TUI で開く |
| `revu validate [dir]` | review.yml と Markdown の整合性チェック |
| `revu status [dir]` | accept/reject の集計、submit 状況を表示 |
| `revu open [dir]` | TUI を起動（cwd の git remote が review.PR.Repo と一致する必要あり） |
| `revu open --repo-root <path> <dir>` | repo 検証をスキップして任意のローカル clone を指定 |
| `revu export [dir] --format json` | 投稿ペイロードを JSON で標準出力（API は呼ばない） |
| `revu submit [dir]` | 投稿フローを起動（`submit` タイプで明示確認） |
| `revu submit --dry-run [dir]` | 投稿内容のプレビュー（API は呼ばない） |
| `revu config` | 現在の設定を表示 |
| `revu config --init` | スターター `config.toml` を書き出す |
| `revu severities` | 有効な severity 一覧を表示（`--json` で機械可読出力、skill が利用） |

`[dir]` を省略すると、cwd の git remote から `~/.revu/{owner}/{repo}/` 配下の最新 `pr-N` を解決します。

## TUI のキーバインド

### グローバル

| キー | 動作 |
|---|---|
| `?` | ヘルプの表示／非表示 |
| `:` | コマンドモード（`:save` / `:quit` / `:submit` / `:reload` / `:filter` など） |
| `Ctrl+S` | 保存 |
| `q` | 終了（未保存変更があれば警告） |

### 一覧画面

| キー | 動作 |
|---|---|
| `j` / `↓` | 下に移動（サマリ行 → コメント） |
| `k` / `↑` | 上に移動（コメント → サマリ行） |
| `Enter` | カーソル位置を開く（サマリ画面 or 詳細画面） |
| `s` | サマリ画面へ直接ジャンプ |
| `/` | フィルタ入力（例: `severity:major,critical category:bug`） |
| `a` / `r` / `u` | コメントを accepted / rejected / pending に変更 |

### 詳細画面

| キー | 動作 |
|---|---|
| `n` / `p` | 次／前のコメント |
| `a` / `r` / `u` | accepted / rejected / pending |
| `e` | `$EDITOR` で `body_file` を編集（保存後に自動再読み込み） |
| `l` | 一覧に戻る |

### サマリ画面

| キー | 動作 |
|---|---|
| `c` | review_event の切り替え（APPROVE / COMMENT / REQUEST_CHANGES） |
| `e` | `$EDITOR` で `summary.md` を編集 |
| `l` | 一覧に戻る |

### コマンドモード（`:`）

| コマンド | 動作 |
|---|---|
| `:save` / `:w` | status の変更を `review.yml` に永続化 |
| `:quit` / `:q` | 終了（未保存変更があれば警告） |
| `:q!` | 強制終了 |
| `:reload` | すべての MD ファイルを再読み込み（fsnotify が無効な環境向け） |
| `:filter <expr>` | 一覧をフィルタ |
| `:filter clear` | フィルタ解除 |
| `:submit` | `revu submit` を起動（dirty 時は `:save` を先に促される） |
| `:submit --dry-run` | プレビューのみ |

### フィルタ式

```
severity:major,critical    重大度（OR within, AND with other dimensions）
category:bug,security      カテゴリ
status:pending             ステータス
path:application.py        ファイルパス部分一致（大文字小文字無視）
```

複数条件は空白区切りで AND 結合されます。

例:
- `severity:major status:pending` — major かつ pending
- `path:auth category:security` — auth を含むパス かつ security カテゴリ

## 設定ファイル

revu は以下の順（優先度の低い → 高い順）で TOML を読み、レイヤーマージします。各キーは上の層で同じキーが出てきたら上書き、出てこなければ下の層の値が残ります。

1. `~/.config/revu/config.toml` — グローバル
2. `<repo-root>/.revu` — プロジェクト共有用（コミット推奨）
3. `<repo-root>/.revu-local` — 個人ローカル用（**`.gitignore` 推奨**）

`<repo-root>` は cwd を起点に `git rev-parse --show-toplevel` で決まります。git リポジトリ外で実行した場合は (1) のみ参照されます。

`$REVU_CONFIG` を設定するとそのパスだけが参照され、他のソースは無視されます（テスト・CI で隔離するための上書き口）。

`revu config` で各ソースの読み込み状態を確認できます:

```text
Sources (lowest → highest priority):
  loaded       /home/.../.config/revu/config.toml
  not present  /path/to/repo/.revu
  loaded       /path/to/repo/.revu-local
```

中身のスキーマはどの層も同じです。すべて任意で、無くても動きます。

```toml
[editor]
# TUI の e キーで使うエディタ。空のとき $EDITOR にフォールバックし、
# それも無いときは vi を使う。
# command = "code --wait"

[ui]
# 詳細画面の左ペインで対象行の前後何行を表示するか
code_context_lines = 5

# 詳細画面が横並びになる端末幅の下限（未満なら縦積み）
horizontal_threshold = 100

[review]
# 新規レビューの review_event 既定値（情報用、現状未使用）
default_event = "COMMENT"

# severity 定義。省略時は組み込みの 4 段階 (critical / major / minor / nit)。
# 1 件でも定義すると組み込みは破棄され、ここに書いた集合だけが有効になる。
# review-pr skill は `revu severities --json` でこの定義を読み取って
# コメント生成と review_event 判定に使う。
#
# [[review.severity]]
# name = "critical"
# level = 100                       # 大きいほど重大
# description = "本番障害・データ破損・重大セキュリティに直結する"
# review_event = "REQUEST_CHANGES"  # APPROVE / COMMENT / REQUEST_CHANGES
# color = "red"
#
# [[review.severity]]
# name = "suggestion"
# level = 40
# description = "改善はするが優先度低、現状でも動く"
# review_event = "COMMENT"
# color = "cyan"
#
# [[review.severity]]
# name = "nit"
# level = 10
# description = "趣味・スタイルの提案、無視されても困らない"
# review_event = "COMMENT"
# color = "gray"
```

`revu config --init` で雛形を書き出せます。

### severity と review_event の対応

各 severity に紐づく `review_event`（`REQUEST_CHANGES` / `COMMENT` / `APPROVE`）が、コメント全体から計算される PR レビューの `review_event` を決めます。skill 側のルール:

1. 各コメントの severity に紐づく `review_event` を集める
2. 一番強いものを採用 — 優先度は `REQUEST_CHANGES` > `COMMENT` > `APPROVE`
3. コメントが 0 件のときは `APPROVE`

例えば `kudos` のような「良かった点」用の severity を `review_event = "APPROVE"` で定義しておけば、その severity だけのコメントは `APPROVE` のままレビューを下げません。

## クローズ/マージ済み PR のレビューを掃除

`revu prune` で `~/.revu/{owner}/{repo}/` 配下の `pr-N/` を走査し、GitHub 上で CLOSED / MERGED の PR に紐づくディレクトリを一括削除できます。OPEN PR や状態取得に失敗した PR は削除されません。

```bash
revu prune                       # cwd リポジトリを対象、確認プロンプト付き
revu prune --repo owner/repo     # 別リポジトリを指定
revu prune --dry-run             # プランの表示のみ
revu prune -y                    # 確認プロンプトをスキップ
```

`submitted_at` が無いレビュー（ローカルで未投稿のもの）は **WARNING 付きで** 削除プランに含まれます。失いたくない作業がある場合は確認プロンプトでキャンセルしてください。

## 投稿フローの安全装置

`revu submit` は次の場合に投稿を中断します:

- `gh auth status` が失敗（未認証）
- review.yml の `head_sha` と PR の現 head が不一致（PR が更新されている）
- `review.yml` に既に `submitted_at` が記録されている（再投稿）
- 確認プロンプトで `submit` 以外を入力した

## ファイル構成

```
~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/
├── review.yml              ← メタデータ + コメント参照（ツールが書き換える）
├── summary.md              ← PR 全体のレビュー本文（人間も編集可）
└── comments/
    ├── c1-...md            ← インラインコメント本文（人間も編集可）
    ├── c2-...md
    └── ...
```

## ライセンス

MIT
