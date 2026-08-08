# revu

Claude Code または OpenAI Codex CLI が生成した PR レビューを TUI で確認・編集し、GitHub に投稿するためのツール。

レビュー本文は YAML + Markdown のハイブリッド構造でローカルに保存され、TUI で取捨選択・推敲してから明示的なコマンドで GitHub に投稿します。

## なぜ revu か

LLM は PR レビューの下書きを定型観点で機械的に流せる一方、生成物をそのまま GitHub に投稿すると質が荒く・誤検出が混じり・責任の所在もあいまいになります。revu は **「AI が下書き → 人間が TUI で取捨選択 → GitHub に投稿」** の中間レイヤーを担い、生成スピードと人間のキュレーションを両立させます。

- **生成は内蔵しない**: Claude Code の revu:pr skill が担当。revu は LLM を持たない
- **ファイルで触れる**: review.yml + Markdown でローカルに置かれ、`$EDITOR` でも `git` でも介入できる
- **投稿は明示確認**: head_sha 不一致や二重投稿などを安全装置で止め、`submit` のタイプ確認を要求する
- **チームで揃えやすい**: 設定・テンプレート・コーディング規約を user / `.revu` / `.revu-local` の 3 層で重ねられる

設計判断の背景や「何を解決しないか」は [docs/why-revu.md](docs/why-revu.md) に詳しく書いています。

## ステータス

開発中（MVP）。Phase 1〜5 完了相当。

## 全体ワークフロー

```
[1] Claude Code / Codex CLI で revu:pr skill を起動
        または
    revu review [PR_NUMBER]            ← cwd リポジトリで自分にレビュー依頼が来ている PR
                                         を選び、内部で claude CLI を起動してレビュー生成
    revu review [PR_NUMBER] --codex    ← claude の代わりに codex CLI で起動
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

> [!TIP]
> このリポジトリ内で Claude Code を開いているなら、`/install` と入力するとプロジェクトスキル（`.claude/skills/install/`）が起動し、以降のバイナリ + Claude Code / Codex プラグインのセットアップを一括で行えます。

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
- `gh` CLI（`revu submit` および `revu:pr` skill で使用、`gh auth login` 済みであること）
- 投稿対象 PR のローカル clone（`revu open` を実行する場所）
- Claude Code または OpenAI Codex CLI（レビュー生成に `revu:pr` skill を使う場合。`revu review` は既定で claude、`--codex` で codex を起動）

## revu プラグイン（スキル）のインストール

スキルはすべて `plugin/` 配下の revu プラグインとして配布します。レビュー生成の `revu:pr` とレビューデータ編集の `revu:edit` の 2 スキル構成で、同じプラグインを Claude Code と OpenAI Codex CLI の両方から呼べます。skill 本体は 1 つで、ランタイムだけ差し替わります。

### Claude Code

`plugin/` を skills-dir プラグインとして `~/.claude/skills/revu` にシンボリックリンクします:

```bash
make install-skills
# アンインストールは make uninstall-skills
```

Claude Code に `/revu:pr <PR_NUMBER>` と入力すると skill が起動し、`~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/` 配下にレビューを書き出します（SHA は PR の `head_sha` 先頭 7 文字）。

```
/revu:pr 123
/revu:pr 123 --focus security,perf
/revu:edit c3 を reject して
```

### OpenAI Codex CLI

Codex CLI 用には、本リポジトリをプラグインマーケットプレース（`.agents/plugins/marketplace.json` → `./plugin`）として登録し、そこから revu プラグインをインストールします。付属スクリプトが両方をまとめて実行します:

```bash
scripts/install-codex.sh
# アンインストールは scripts/install-codex.sh --uninstall
```

Codex はプラグインを `~/.codex/plugins/cache/` に**コピー**するため、リポジトリを更新したらスクリプトを再実行してください。インストール後 Codex を再起動すると、対話プロンプトで `$revu:pr <PR_NUMBER>` が使えるようになります:

```
$revu:pr 123
$revu:pr 123 --focus security,perf
```

`revu review --codex` はこの skill を `codex exec --json` 経由で非対話的に起動します。

#### Codex 起動時に revu が上書きする設定

`revu review --codex` が `codex exec` を起動するとき、以下の `-c` 上書きを per-invocation で渡します。いずれも **この exec の間だけ** 有効で、`~/.codex/config.toml` の値は別の codex 実行には残ります。

| 上書き | 理由 |
|---|---|
| `sandbox_workspace_write.network_access=true` | `workspace-write` sandbox は既定で外向き通信を遮断するため、`gh pr view` / `gh pr diff` が api.github.com に届かない |
| `model_reasoning_effort="high"` | 既定の `medium` だと codex が skill の「5〜10件目安」を無視して「1件に絞る」と宣言しがちなため、PR 全文を読み通す予算を確保する |

さらに、revu は codex に送るプロンプト末尾に **「自主的に 1 件まで縮減しないこと」** という指示を 1 行付けます (claude には付けません)。これは codex の exec モードの過剰な簡略化バイアスを打ち消すためです。`--focus` によるカテゴリ絞り込みはそのまま尊重されます。

skill が完了したら `revu open` で開けます。

### `revu review` で claude を使うときの permission

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

## 非対話モード（CI / 自動化から呼ぶ）

既定の `revu review` はレビュー生成後に必ずエージェントの対話 TUI へ入るため、CI やバックグラウンドワーカーからは端末を掴んだまま戻ってきません。`--no-resume` を付けると **生成が終わった時点でプロセスが終了** します。

```bash
# 生成して終了。どこに出力されたかを人間が読める形で表示
revu review 42 --no-resume

# 生成して終了。結果を JSON で標準出力（進捗は標準エラーへ）
revu review 42 --codex --no-resume --json
```

`--json` の出力:

```json
{
  "engine": "codex",
  "repo": "ystsbry/revu",
  "pr": 42,
  "out_dir": "/home/you/.revu/ystsbry/revu/pr-42/a1b2c3d",
  "session_id": "0199..."
}
```

このフィールド名は消費側との契約なので、互換性を保って変更します。`session_id` は `--codex` のとき codex の `thread_id` が入ります（`review.yml` の `generated_by` と同じ扱い）。

**エージェントが session_id を返さなかった場合、`session_id` キー自体が出力から消えます**（レビューの生成自体は成功しているので終了コードは 0）。その実行は `revu resume` できないため、標準エラーに警告を出します。消費側は `session_id` の有無を確認してから resume 系の処理につないでください。

| 挙動 | 内容 |
|---|---|
| `--json` は `--no-resume` が必須 | 結果 JSON を出した直後に端末をエージェントへ渡すと、呼び出し側が標準出力を追えなくなるため |
| PR 番号は必須 | 非対話モードでは対話ピッカーへ落ちず、番号を省略すると明確なエラーで終了する |
| 標準出力の分離 | `--json` のときは結果 JSON のみが標準出力に出る。エージェントの進捗・revu のステータス行・警告・エージェント自身の stderr はすべて標準エラーへ |
| stdin | エージェントには空の stdin を渡すため、TTY の無い環境でも入力待ちでハングしない |
| 失敗時 | レビューが生成されなかった場合は非ゼロで終了する。**今回の実行で書かれていないレビュー dir は「不在」として扱う**ので、過去の実行結果を新しい成果物として掴むことはない |
| `generated_by` | `tool` / `session_id` の review.yml への書き戻しは resume の有無に関わらず従来どおり行われる |

**対象リポジトリは cwd の git remote から解決されます。** `revu pr prepare` も `gh` も `codex --cd` も cwd を対象にするため、CI は必ず checkout の中から `revu` を呼んでください（`--repo` のようなフラグは、実際にレビューされるリポジトリを変えられないので用意していません）。

既定の `revu review`（フラグなし）の挙動は従来から変わりません。

## テンプレートのカスタマイズ

`revu:pr` skill が生成するサマリとインラインコメントの構造はテンプレートで決まっています。デフォルトは `~/.claude/skills/revu/skills/pr/templates/` 配下:

- `summary.md.tmpl` — PR 全体サマリ
- `inline-comment.md.tmpl` — 各インラインコメント

**ユーザー上書き** は設定レイヤーと同じ場所の `templates/` 配下に同名ファイルを置くと適用されます。優先度は高い順に:

1. `<repo>/.revu-local/templates/<NAME>` — 個人ローカル（`.gitignore` 推奨）
2. `<repo>/.revu/templates/<NAME>` — プロジェクト共有（コミット推奨）
3. `$REVU_TEMPLATES/<NAME>` — env が立っているとき
4. `~/.config/revu/templates/<NAME>` — グローバル

skill は `revu templates path <NAME>` を呼んで上記を解決し、いずれにもヒットしなければ skill 同梱にフォールバックします。

```bash
# プロジェクトでサマリだけ揃えたいケース
mkdir -p .revu/templates
cp ~/.claude/skills/revu/skills/pr/templates/summary.md.tmpl .revu/templates/
# ↑ お手本としてコピーしてから編集する
```

`revu templates list` で現在解決される一覧を確認できます。テンプレートはあくまで「お手本」で、Claude が構造ガイドとして参照するだけです。固定の文字列置換ではありません。

## レビュー指針（コーディング規約など）の追加

skill 同梱の観点表（bug / design / style / perf / security / test / doc）に加えて、プロジェクトやチーム固有のレビュー指針を Markdown ファイルで渡せます。`config.toml` の `[review] guidelines` にパスを並べると、レビュー時に skill がそれらを読み込んで観点に加えます。

```toml
# <repo>/.revu/config.toml
[review]
guidelines = [
  "guidelines/coding-style.md",
  "guidelines/security-checklist.md",
]
```

- パスは **その config.toml からの相対** で解決されます（絶対パスも可）
- レイヤー（user → `.revu` → `.revu-local`）で **連結** されるので、グローバル規約とプロジェクト固有規約を併用できます
- 存在しないパスは `revu guidelines list` で MISSING 表示、`revu guidelines paths` では除外（skill は欠落を黙殺してレビュー継続）

```text
$ revu guidelines list
  #  STATUS   PATH
  1  OK       /home/.../.config/revu/guidelines/personal.md
  2  OK       /repo/guidelines/coding-style.md
  3  MISSING  /repo/guidelines/security-checklist.md
```

ガイドラインに書かれた具体的なルールは、レビューコメントの根拠として参照されます（「`coding-style.md` の "命名" 節に従い ...」のような形）。

## コマンド一覧

| コマンド | 用途 |
|---|---|
| `revu version` | バージョン表示 |
| `revu review [PR_NUMBER]` | 自分にレビュー依頼が来ている PR を選び、`claude` CLI で `/revu:pr` を実行して生成された結果を TUI で開く |
| `revu review [PR_NUMBER] --codex` | 同上だが `claude` の代わりに `codex` CLI で `$revu:pr` skill を起動（`scripts/install-codex.sh` でプラグインのインストールが必要） |
| `revu review PR_NUMBER --no-resume` | レビューを生成した時点で終了し、対話 TUI に入らない（[非対話モード](#非対話モードci--自動化から呼ぶ)） |
| `revu review PR_NUMBER --no-resume --json` | 同上で、結果を JSON で標準出力（進捗は標準エラーへ） |
| `revu validate [dir]` | review.yml と Markdown の整合性チェック |
| `revu status [dir]` | accept/reject の集計、submit 状況を表示 |
| `revu open [dir]` | TUI を起動（clone は cwd 一致 → 登録リポジトリの順で解決） |
| `revu open --repo-root <path> <dir>` | repo 検証をスキップして任意のローカル clone を指定 |
| `revu export [dir] --format json` | 投稿ペイロードを JSON で標準出力（API は呼ばない） |
| `revu submit [dir]` | 投稿フローを起動（`submit` タイプで明示確認） |
| `revu submit --dry-run [dir]` | 投稿内容のプレビュー（API は呼ばない） |
| `revu submit --no-approve [dir]` | `review_event: APPROVE` のレビューを COMMENT に降格して投稿（CI 向け。COMMENT / REQUEST_CHANGES は変わらない） |
| `revu repo scan <root>` | ディレクトリ走査で clone を検出し、user config の `[[repo]]` へ一括登録（`--dry-run` あり） |
| `revu repo add <path>` | clone を 1 つ登録（既存 slug はパス更新） |
| `revu repo list` | 登録リポジトリの一覧（パス消失は `(missing)` 表示） |
| `revu repo remove <slug>` | 登録を削除 |
| `revu profile list` | プロファイル一覧と active の表示 |
| `revu profile use <name>` | プロファイルを切り替え（`default` で解除） |
| `revu config` | 現在の設定を表示 |
| `revu config --init` | スターター `config.toml` を書き出す |
| `revu severities` | 有効な severity 一覧を表示（`--json` で機械可読出力、skill が利用） |

`[dir]` を省略すると、cwd の git remote から `~/.revu/{owner}/{repo}/` 配下の最新 `pr-N` を解決します。`validate` / `status` / `export` / `open` / `resume` は `--repo <owner>/<repo>` でも解決でき、この場合 cwd がどこであっても（git リポジトリの外でも）動作します。

## リポジトリ登録（cwd 非依存の解決）

`revu repo scan / add` で「slug ↔ ローカル clone パス」を登録しておくと、cwd に依存しない解決ができるようになります。ghq を使っているなら root を一度走査すれば十分です:

```bash
revu repo scan ~/ghq            # 検出結果を登録（--dry-run で確認だけも可）
revu repo list
```

- 登録先はグローバル user config（`os.UserConfigDir()/revu/config.toml`）の `[[repo]]` ブロック
- revu の機械編集は **`[[repo]]` ブロックだけ**を追記・置換・削除し、それ以外のコメント・設定はそのまま保持する
- 登録済みの clone は `revu open` の clone 解決（cwd 不一致時のフォールバック）と、今後のダッシュボード機能で利用される

### プロファイル（登録リポジトリの絞り込み）

全件を登録したうえで、名前付きのサブセット「プロファイル」に分けられます。プロファイルは config に手で宣言し、切り替えは `revu profile use` で行います:

```toml
# config.toml
[[profile]]
name = "work"
repos = ["acme/api", "acme/web"]

[[profile]]
name = "oss"
repos = ["ystsbry/revu"]
```

```bash
revu profile list           # 宣言済みプロファイルと active の確認
revu profile use work       # 以後 repo list / ダッシュボードは work の 2 リポジトリだけ表示
revu profile use default    # 解除（全件表示に戻る）
revu repo list --all        # active profile を無視して一時的に全件表示
revu repo list --profile oss  # 一時的に別プロファイルで表示
```

- `default` は予約名で「登録済み全件」を意味する（`[[profile]]` として宣言はできない）
- 選択は user config の `active_profile` キーとして永続化される（`[[repo]]` と同じく、このキーの行だけを機械編集する）
- プロファイルが参照する slug が未登録の場合は黙って落とさず、`repo list` / `profile list` が明示する

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

revu は「設定レイヤー」をディレクトリ単位で持ちます。各レイヤーは `config.toml` と `templates/` を同居でき、優先度の低い → 高い順に重ねた結果が有効値になります。

1. `~/.config/revu/` — グローバル
2. `<repo-root>/.revu/` — プロジェクト共有用（コミット推奨）
3. `<repo-root>/.revu-local/` — 個人ローカル用（**`.gitignore` 推奨**）

各ディレクトリの中身:

```
~/.config/revu/
├── config.toml               # 任意。設定 TOML
└── templates/                # 任意。テンプレートの上書き
    ├── summary.md.tmpl
    └── inline-comment.md.tmpl
```

`<repo-root>` は cwd を起点に `git rev-parse --show-toplevel` で決まります。git リポジトリ外で実行した場合は (1) のみ参照されます。

`config.toml` のキーは上のレイヤーで同じキーが出てきたら上書き、出てこなければ下のレイヤーの値が残ります。`$REVU_CONFIG` を設定するとそのパスだけが参照され、他のソースは無視されます（テスト・CI で隔離するための上書き口）。

`revu config` で各ソースの読み込み状態を確認できます:

```text
Sources (lowest → highest priority):
  loaded       /home/.../.config/revu/config.toml
  not present  /path/to/repo/.revu/config.toml
  loaded       /path/to/repo/.revu-local/config.toml
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
# revu:pr skill は `revu severities --json` でこの定義を読み取って
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

### CI から投稿する場合（`--no-approve`）

GitHub は Actions の `GITHUB_TOKEN` による PR 承認を禁じています。`review_event: APPROVE` のレビューをそのまま投稿すると HTTP 422（`GitHub Actions is not permitted to approve pull requests.`）で API 呼び出しごと失敗し、レビューが 1 件も残りません。指摘が nit / minor だけに収まった「出来のよい PR」ほどこの状態になります。

`--no-approve` を付けると、APPROVE のレビューを COMMENT に降格して投稿します。

```bash
revu submit --yes --accept-pending --no-approve "$dir"
```

- COMMENT / REQUEST_CHANGES のレビューは何も変わりません（フラグを付けても投稿内容は同じ）
- 降格したときは実行ログに `Downgraded review event: APPROVE -> COMMENT (--no-approve)` を出力し、投稿前プレビューの `Event:` 行も降格後の値（`COMMENT`）になります
- 投稿されるレビュー本文（`summary.md` の内容）は書き換えません
- 投稿成功時に `review.yml` へ書き戻される `review_event` は、実際に投稿した `COMMENT` になります

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
