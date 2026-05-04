---
name: review-pr
description: GitHub PR を読解し、revu が消費できる形式 (~/.revu/{owner}/{repo}/pr-{N}/) に review.yml + summary.md + comments/*.md を生成する。「PR をレビューして」「review-pr」「PR #123 のレビューを下書きして」などと言われたら使う。引数は <PR_NUMBER> と任意の --focus <categories>。
---

# review-pr

PR にレビューコメントを生成し、`revu` が読み込める形式に書き出す。

## 入力

```
/review-pr <PR_NUMBER>
/review-pr <PR_NUMBER> --focus <categories>
```

- `<PR_NUMBER>` (必須): GitHub の PR 番号
- `--focus <categories>` (任意): レビュー観点を絞る。カテゴリのカンマ区切り (`security,perf` など)。指定しない場合は全観点

## 出力

以下のディレクトリ構造を `~/.revu/{owner}/{repo}/pr-{N}/` 配下に生成:

```
~/.revu/{owner}/{repo}/pr-{N}/
├── review.yml          ← メタデータ + コメント参照配列
├── summary.md          ← PR 全体のレビューコメント本文
└── comments/
    ├── c1-{filename}-{line}.md            ← 単一行
    ├── c2-{filename}-{start}-{end}.md     ← 範囲
    └── ...
```

`{owner}/{repo}` は PR のベースリポジトリ、`{filename}` は対象ファイルのベース名（拡張子なし、英数字に正規化）。

## 手順

以下を順番に実行する。

### 1. 引数パース

`<PR_NUMBER>` を整数として取り出す。`--focus` があればカテゴリ集合として保持。

### 2. PR 情報取得 + 出力先準備

```bash
revu pr prepare <PR_NUMBER>
```

これは内部で `gh pr view` と `mkdir -p` を実行する `revu` のヘルパ。1 回の呼び出しで JSON が返る:

```json
{
  "repo": "ystsbry/revu",
  "number": 123,
  "head_sha": "abc...",
  "base_branch": "main",
  "title": "...",
  "body": "...",
  "review_dir": "/home/<user>/.revu/ystsbry/revu/pr-123"
}
```

返り値から以下を抽出:
- `repo` → review.yml の `pr.repo`
- `head_sha` → `pr.head_sha`
- `base_branch` → `pr.base_branch`
- `title`, `body` → 文脈把握に使う
- `review_dir` → 以降の出力先（`comments/` も既に作成済み）

### 3. PR diff 取得

```bash
revu pr diff <PR_NUMBER>
```

これがレビュー対象。内部で `gh pr diff` を呼ぶ。**diff 全体を必ず通読してから**コメントを書き始める。部分的に見て指摘するな。

### 4. テンプレート解決

ユーザー上書きがあればそちらを優先する。無ければ skill 同梱を使う。

**サマリのテンプレート解決順:**
1. `~/.config/revu/templates/summary.md.tmpl` (存在すれば)
2. `~/.claude/skills/review-pr/templates/summary.md.tmpl` (skill 同梱)

**インラインコメントのテンプレート解決順:**
1. `~/.config/revu/templates/inline-comment.md.tmpl` (存在すれば)
2. `~/.claude/skills/review-pr/templates/inline-comment.md.tmpl` (skill 同梱)

`$REVU_TEMPLATES` 環境変数が設定されていれば、`~/.config/revu/templates/` の代わりにそれを使う。

`Read` ツールで存在確認しつつ、ヒットした方の内容を構造ガイドとして以降の生成に使う。テンプレートはあくまで「お手本」。固定文字列の置換ではない。

### 5. severity セットの解決

severity 名・level・review_event は revu の設定で決まる。**ハードコードしてはいけない**。
以下を実行して、出力 JSON を以降のコメント生成・review_event 判定に使う:

```bash
revu severities --json
```

各エントリの形:

```json
{
  "name": "critical",
  "level": 100,
  "description": "...",
  "review_event": "REQUEST_CHANGES",
  "color": "red"
}
```

- `name` を `review.yml` の `comments[].severity` および各コメント Markdown の見出し `## [{severity}] ...` に使う
- `level` は数値で大きいほど重大。指摘の重大度を判定するときの目安
- `description` は各 severity の意図。観点を絞るときの参考にする
- `review_event` は Step 10 の review_event 判定に使う

`revu` が `$PATH` に無い場合は、デフォルトの severity セットで進める:

| severity | level | review_event | 定義 |
|---|---|---|---|
| `critical` | 100 | REQUEST_CHANGES | 本番障害・データ破損・セキュリティ重大インシデントになり得る |
| `major` | 80 | REQUEST_CHANGES | 設計の根本問題、リファクタが必要、将来のバグ温床 |
| `minor` | 30 | COMMENT | 改善はするが優先度低、現状でも動く |
| `nit` | 10 | COMMENT | 趣味・スタイルの提案、無視されても困らない |

### 6. diff の読解とコメント生成

PR の差分から、以下の観点で改善点を洗い出す。`--focus` 指定があれば該当カテゴリに限定:

| カテゴリ | 観点 |
|---|---|
| `bug` | 実行時エラー、null 参照、競合状態、エッジケース漏れ |
| `design` | レイヤー違反、責務分離、命名、抽象化レベル |
| `style` | 命名規約違反、フォーマット、慣用句逸脱 |
| `perf` | N+1、不要な確保、ホットパスでの非効率 |
| `security` | 認可漏れ、入力検証不足、シークレット露出 |
| `test` | 網羅性不足、脆いテスト、欠落テスト |
| `doc` | 不正確 / 古い / 不足するドキュメント |

各指摘の severity は **Step 5 で取得した定義集合**から選ぶ。`description` を読んで対応する `name` を割り当て、`level` の数値感も判断材料にする。

**重要な指針:**
- **数より質**: 1 PR で 5〜10 件を目安。20 件超は出さない。重要度の低い指摘で埋めない
- **根拠を書く**: なぜそれが問題かを具体的に。一般論で終わらせない
- **代替案を示す**: 改善が示せるなら GitHub suggestion ブロック (` ```suggestion` ) を使う
- **既存コードの慣習を尊重**: 周辺コードと矛盾するスタイル提案はしない
- **指摘範囲は対象に合わせる**: 指摘が連続する複数行に跨るなら、単一行ではなく **範囲指定** (`start_line` + `line`) を使う。GitHub 上でも該当ブロック全体がハイライトされ、レビュー意図が伝わりやすい。範囲指定を検討すべき典型例:
  - 関数 / コンポーネント / hook 全体の設計問題（例: `useCallback` の宣言から閉じ括弧まで）
  - 多行に渡る依存配列・リテラル・型定義
  - if/switch ブロックや try/catch ブロック全体に対する指摘
  - 連続するインポート群、連続する代入文
  - YAML / JSON のキーから値ブロック全体（multi-line block）

  逆に**ピンポイント**（typo、1 行の null 参照、1 行の比較演算子ミス等）は単一行のままでよい。

### 7. インラインコメントを書き出す

各指摘について `$REVIEW_DIR/comments/c{N}-{基底名}-{lines}.md` に書き出す。`{N}` は 1 始まり。`{基底名}` はファイル名から拡張子を取り、英数字以外を `-` に置換したもの。`{lines}` の規則:

- 単一行コメント (`line` のみ) → `{line}`
- 範囲コメント (`start_line` + `line`) → `{start_line}-{line}`

例:
- `src/features/order/application.py:42` への c1 → `c1-application-42.md`
- `src/features/order/application.py:42-58` への c2 → `c2-application-42-58.md`

**ファイル名の数字と `review.yml` の行番号は必ず一致させること**。範囲を意図しているのにファイル名は範囲、`review.yml` は単一行、といったズレは禁止。レビュー再生成時の混乱の元になる。

各 MD のフォーマットは **Step 4 で解決したテンプレート**に従う。テンプレートが与える構造（見出し、提案ブロックの位置、参考セクション等）を踏襲しつつ、内容は今回の diff 固有のものを書く。

**見出し**は `## [{severity}] 見出し本文` の形にする。例:

```
## [major] OrderRepository を直接呼ばず UnitOfWork 経由にすべき
```

`{severity}` は Step 5 で解決した severity セットの `name` のいずれか。`review.yml` の `comments[].severity` と必ず一致させること。GitHub に投稿された後、コメント本文だけ読んでも重大度が分かるようにするための表記。

### 8. summary.md を書き出す

`$REVIEW_DIR/summary.md` に PR 全体のサマリを書く。**Step 4 のサマリテンプレート**に従う。

サマリには次を含める:
- 全体所感（何をしている PR か、設計の妥当性）
- 良かった点
- 改善が必要な点（インラインコメントの ID `c1`, `c4` などを参照）

### 9. review.yml を書き出す

`$REVIEW_DIR/review.yml` を以下のフォーマットで書く:

```yaml
schema_version: 1

pr:
  repo: {owner}/{repo}
  number: {PR_NUMBER}
  head_sha: {headRefOid}
  base_branch: {baseRefName}

generated_at: {ISO8601 形式の現在時刻}
generated_by:
  tool: claude-code
  skill: review-pr
  model: claude-opus-4-7

review_event: {APPROVE | COMMENT | REQUEST_CHANGES}
summary_file: summary.md

comments:
  # 単一行コメント
  - id: c1
    status: pending
    severity: {Step 5 の severity name のいずれか}
    category: {bug | design | style | perf | security | test | doc}
    path: {リポジトリ相対パス}
    line: {行番号}
    side: RIGHT
    body_file: comments/c1-{基底名}-{line}.md

  # 範囲コメント（連続する複数行に跨る指摘）
  - id: c2
    status: pending
    severity: ...
    category: ...
    path: ...
    start_line: {開始行}
    line: {終了行}
    side: RIGHT
    body_file: comments/c2-{基底名}-{start_line}-{line}.md

  - id: c3
    ...
```

**フィールド詳細:**
- `pr.repo` / `pr.head_sha` / `pr.base_branch`: Step 2 で取得した値をそのまま
- `generated_at`: `revu now` を実行して得た ISO 8601 タイムスタンプを使う
- `comments[].id`: `c1`, `c2`, ... の連番
- `comments[].status`: 必ず `pending`（ユーザーが TUI で取捨選択する）
- `comments[].side`: 追加・変更行へのコメントは `RIGHT`、削除行は `LEFT`。基本は `RIGHT`
- `comments[].path`: PR ベースリポジトリのルートからの相対パス
- `comments[].line`: 単一行のときはその行、範囲のときは**終了行**（GitHub のスキーマ準拠）
- `comments[].start_line`: **範囲指定するときに必須**。Step 6 のガイドラインに従い、指摘が複数行に跨るなら省略せず付ける。同一 side のときは `start_side` を省略してよい（自動的に `side` と同じ扱い）。**削除行 (`-`) と追加行 (`+`) を跨いで範囲指定したいとき** は `start_side: LEFT` + `side: RIGHT` のように両方を明示する（同一 side では `start_line <= line` 制約あり、跨ぐ場合は制約なし）

### 10. review_event の判定

サマリと全インラインを書き終えてから、以下のルールで `review_event` を決める:

- 各コメントの severity に紐づく `review_event`（Step 5 の JSON で取得済）を集める
- 集めた中で**最も強いもの**を採用する。優先度は `REQUEST_CHANGES` > `COMMENT` > `APPROVE`
- コメントが 0 件のときは `APPROVE`

例（デフォルト severity セットの場合）:
- `critical` × 1 + `nit` × 5 → `REQUEST_CHANGES`（critical が REQUEST_CHANGES なので）
- `minor` × 3 → `COMMENT`
- 0 件 → `APPROVE`

ユーザーは TUI で `c` キーで変更できるので、初期値の妥当性は完璧でなくて良い。

### 11. 出力の自検

最後に `revu validate` で生成物のスキーマ整合性を確認する:

```bash
revu validate "$REVIEW_DIR"
```

成功したらユーザーに報告:

```
Generated review at ~/.revu/{owner}/{repo}/pr-<N>/
- summary.md
- {N} inline comments
- review_event: {APPROVE | COMMENT | REQUEST_CHANGES}

Open with: revu open
```

`revu` が `$PATH` に無い場合は警告を出すだけで続行（インストール案内を添える）。

`revu validate` がエラーを出した場合は、出力を読んで修正する。よくある原因:
- enum 値の typo (`major` を `Major` と書いた等)
- `body_file` のパスが存在しない
- `id` の重複

## 注意事項

- **書き換えてはいけないもの**: ユーザーの既存 `~/.revu/{owner}/{repo}/pr-<N>/` に `submitted_at` が記録されている場合、再生成は別ディレクトリ（`pr-<N>-r2` など）に書くか、ユーザーに上書き確認を取る
- **diff 外の変更には触れない**: PR に含まれない既存コードへの不満は書かない（PR の責務外）
- **過度な依存提案は避ける**: 「このライブラリに置き換えるべき」は major 以上の確信があるときのみ
- **言語**: コメント本文は日本語。ただし code suggestion 内は当該言語
