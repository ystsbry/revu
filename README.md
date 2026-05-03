# revu

Claude Code が生成した PR レビューを TUI で確認・編集し、GitHub に投稿するためのツール。

レビュー本文は YAML + Markdown のハイブリッド構造でローカルに保存され、TUI で取捨選択・推敲してから明示的なコマンドで GitHub に投稿します。

## ステータス

開発中（MVP）。Phase 1〜4 完了相当。

## 全体ワークフロー

```
[1] Claude Code の review-pr skill が PR レビューを生成
        ↓
[2] ~/.revu/{owner}/{repo}/pr-{N}/ に review.yml + summary.md + comments/*.md が出力される
        ↓
[3] revu open で TUI を起動
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

## 必要なもの

- Go 1.23 以上
- `gh` CLI（`revu submit` でのみ必要、`gh auth login` 済みであること）
- 投稿対象 PR のローカル clone（`revu open` を実行する場所）

## コマンド一覧

| コマンド | 用途 |
|---|---|
| `revu version` | バージョン表示 |
| `revu validate [dir]` | review.yml と Markdown の整合性チェック |
| `revu status [dir]` | accept/reject の集計、submit 状況を表示 |
| `revu open [dir]` | TUI を起動（cwd の git remote が review.PR.Repo と一致する必要あり） |
| `revu open --repo-root <path> <dir>` | repo 検証をスキップして任意のローカル clone を指定 |
| `revu export [dir] --format json` | 投稿ペイロードを JSON で標準出力（API は呼ばない） |
| `revu submit [dir]` | 投稿フローを起動（`submit` タイプで明示確認） |
| `revu submit --dry-run [dir]` | 投稿内容のプレビュー（API は呼ばない） |
| `revu config` | 現在の設定を表示 |
| `revu config --init` | スターター `config.toml` を書き出す |

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

`~/.config/revu/config.toml`（`$REVU_CONFIG` で上書き可能）。すべて任意で、無くても動きます。

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
```

`revu config --init` で雛形を書き出せます。

## 投稿フローの安全装置

`revu submit` は次の場合に投稿を中断します:

- `gh auth status` が失敗（未認証）
- review.yml の `head_sha` と PR の現 head が不一致（PR が更新されている）
- `review.yml` に既に `submitted_at` が記録されている（再投稿）
- 確認プロンプトで `submit` 以外を入力した

## ファイル構成

```
~/.revu/{owner}/{repo}/pr-{N}/
├── review.yml              ← メタデータ + コメント参照（ツールが書き換える）
├── summary.md              ← PR 全体のレビュー本文（人間も編集可）
└── comments/
    ├── c1-...md            ← インラインコメント本文（人間も編集可）
    ├── c2-...md
    └── ...
```

## ライセンス

MIT
