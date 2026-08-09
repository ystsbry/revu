---
name: read
description: revu 形式で生成されたレビュー結果（review.yml + summary.md + comments/*.md）を読み、人間が読みやすい形で提示する。PR 番号を省略した場合は「GitHub で open かつローカルにレビュー結果がある PR」を候補としてユーザーに確認する。「レビュー結果を読んで」「PR 123 のレビュー内容を見せて」「revu:read」などと言われたら使う。読み取り専用（編集は revu:edit）。
---

# revu:read

生成済みのレビュー結果を読み、整形して提示する。**読み取り専用**の skill であり、ファイルは一切変更しない（編集は `revu:edit` の仕事）。

## 入力

```
/revu:read [PR_NUMBER]
```

- `<PR_NUMBER>` (任意): 読む対象の PR 番号。省略時は Step 1b で候補から選んでもらう

## 前提

- cwd のリポジトリを対象とする（`revu` が cwd の git remote から `~/.revu/{owner}/{repo}/` を解決する）
- GitHub の PR 状態（open かどうか）の取得は **`gh` を直接呼ばず、`revu prune --dry-run` を使う**（revu が内部で問い合わせる）

## 手順

### 1a. PR 番号が渡された場合

まず引数なしで実行して、cwd のリポジトリのレビュー置き場を得る:

```bash
revu validate
```

出力 `OK /home/.../.revu/{owner}/{repo}/pr-{M}/{sha} (...)` から `~/.revu/{owner}/{repo}` の部分（リポジトリ base）を取り出し、対象 PR の dir を検証する:

```bash
revu validate <base>/pr-<PR_NUMBER>
```

- 成功したら `OK <dir> (PR #N, M comments: pending=... accepted=... rejected=... edited=...)` の `<dir>` が読む対象。件数もこの行から控える
- **dir が存在しないエラー**の場合はその PR のローカルレビューが存在しない。Step 1b の候補提示に切り替えるか、`revu review <N>` / `/revu:pr <N>` での生成を案内する
- **`invalid severity ...` などのバリデーションエラー**の場合、レビューは存在するが現在の config の severity 定義（レイヤーで差し替え可能）と合っていない。この skill は読むだけなので中断せず、`<base>/pr-<N>/` 配下で最も新しい `{sha}` サブディレクトリ（`review.yml` を含むもの）を読む対象とし、提示時に「現在の severity 設定と不整合（当時の定義で生成）」と警告を添える

### 1b. PR 番号が渡されなかった場合

「GitHub で open、かつローカルにレビュー結果がある PR」を revu で取得する:

```bash
revu prune --dry-run
```

- `Skipped (open, N): pr-700, pr-715` の行が **open かつローカルレビューあり** の一覧。この番号群を候補としてユーザーに提示し、どれを読むか確認する
- ヘッダ行 `Inspected ... under <RepoDir> (slug=...)` の `<RepoDir>` が Step 1a の base に相当するので、選ばれた番号で `revu validate <RepoDir>/pr-<N>` を実行して dir を得る
- `Skipped (open, ...)` の行が無い（open × ローカルありが 0 件）場合はその旨を伝えて終了する。`To delete` に並ぶ merged/closed の番号は既に閉じた PR なので、ユーザーが明示的に望んだ場合のみ対象にする
- `Skipped (state query failed, ...)` に載った PR は状態不明。候補に含める場合はその旨を添える

### 2. レビュー結果の読み込み

Step 1 で得た `<dir>` 配下を `Read` ツールで読む:

1. `<dir>/review.yml` — メタデータ（pr.*、review_event、submitted_at、comments 配列）
2. `<dir>/summary.md` — PR 全体サマリ
3. `comments[].body_file` が指す各 Markdown — インラインコメント本文

### 3. 提示

以下の構成で、人間が読みやすい形に整形して提示する:

```
## {repo} #{PR} のレビュー（{sha[:7]}）

- review_event: {APPROVE|COMMENT|REQUEST_CHANGES}
- 投稿状況: submitted_at があれば日時、無ければ「未投稿」
- コメント: {N} 件（pending=… accepted=… rejected=… edited=…）

### サマリ
（summary.md の内容）

### インラインコメント
#### c1 [severity/category] path:line（status）
（本文）
...
```

- コメントは review.yml の並び順で提示する
- コメントが多く本文が長大な場合（目安: 合計で数百行を超える）は、各コメントを「見出し + 要旨 1〜2 行」に圧縮した一覧を先に出し、全文が必要なら個別に展開できると案内する
- `rejected` のコメントも省略せず表示する（status がひと目で分かるように）

## 注意事項

- **ファイルを変更しない**。status 変更・本文修正の要望が出たら `revu:edit` に切り替える
- `revu prune --dry-run` は状態確認のみでファイル削除は行わないが、`--dry-run` を付け忘れると削除確認プロンプトに進むため必ず付けること
- cwd が対象リポジトリの外にある場合は、ユーザーに slug を確認して `~/.revu/<owner>/<repo>` を base として同じ手順を踏む（この場合 Step 1b の open 判定は `revu prune --dry-run --repo <slug>` を使う）
