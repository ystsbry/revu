---
name: delete
description: revu 形式で生成されたレビュー結果（~/.revu/{owner}/{repo}/pr-{N}/ 配下）を安全に削除する。PR 番号を省略した場合はローカルのレビュー一覧を GitHub 状態（open/merged/closed）付きで提示して確認する。未投稿・open PR のレビューは警告し、ユーザーの明示的な確認なしには削除しない。「レビュー結果を消して」「PR 123 のレビューを削除」「revu:delete」などと言われたら使う。
---

# revu:delete

生成済みのレビュー結果を、対象と影響を明示したうえで削除する。**破壊的操作の skill** なので、必ずユーザーの明示的な確認を取ってから消す。

閉じた PR（merged / closed）の**一括**掃除が目的なら、この skill ではなく `revu prune` を案内する（状態ベースの一括削除は revu 本体の機能）。この skill は「この PR のレビューを消したい」という**個別削除**を担う。

## 入力

```
/revu:delete [PR_NUMBER ...]
```

- `<PR_NUMBER>` (任意、複数可): 削除対象の PR 番号。省略時は Step 1b で候補から選んでもらう

## 前提

- cwd のリポジトリを対象とする。cwd 外のリポジトリは slug を確認し、`revu prune --dry-run --repo <slug>` と `~/.revu/<owner>/<repo>` を base として同じ手順を踏む
- GitHub の PR 状態の取得は `gh` を直接呼ばず `revu prune --dry-run` を使う（revu が内部で問い合わせる）

## 手順

### 1a. PR 番号が渡された場合

リポジトリ base と対象 dir を解決する:

```bash
revu validate
```

出力 `OK /home/.../.revu/{owner}/{repo}/pr-{M}/{sha} (...)` から `~/.revu/{owner}/{repo}` を base として取り出す。対象 dir は `<base>/pr-<N>`。存在しなければその旨を伝えて終了する。

あわせて GitHub 状態も取得しておく（Step 3 の警告に使う）:

```bash
revu prune --dry-run
```

### 1b. PR 番号が渡されなかった場合

```bash
revu prune --dry-run
```

- ヘッダ `Inspected ... under <RepoDir> (slug=...)` の `<RepoDir>` が base
- `To delete (N):` 配下（merged / closed）と `Skipped (open, N): pr-A, pr-B`（open）を合わせた**全ローカルレビュー**を、状態付きの一覧としてユーザーに提示し、どれを削除するか確認する
- `WARNING: contains unsubmitted reviews` の印が付いた PR はその旨も一覧に含める
- ローカルレビューが 1 件も無ければその旨を伝えて終了する

### 2. 対象の内容確認

削除対象それぞれについて、消えるものを把握して提示する:

```bash
revu status <base>/pr-<N>
```

- コメント件数・accept/reject の内訳・投稿状況（`Submitted:`）を控える
- 現在の severity 設定と不整合の古いレビューではこのコマンドが `invalid severity ...` で失敗することがある。その場合は `ls <base>/pr-<N>` で SHA dir 構成だけ示し、「内容の詳細は取得不可（旧 severity 定義で生成）」と添える

### 3. 削除前の最終確認（必須）

以下を明示してから、**削除してよいかユーザーに確認する**。確認なしに `rm` を実行してはならない:

- 削除する dir の絶対パス（`pr-<N>` 全体。特定の SHA dir だけ消したい意図が示されたときはその dir）
- GitHub 状態: **open の PR のレビューを消す場合は強調して警告**（レビュー作業が失われる。再生成は可能だが編集内容は戻らない）
- 投稿状況: **`Submitted: not yet` かつコメントがある場合は「未投稿の作業が失われる」と強調**。投稿済み（submitted_at あり）なら GitHub 上の投稿は消えないことを添える
- 複数 PR を対象にする場合は一覧全体を見せて一括で確認を取る

### 4. 削除の実行

確認が取れた対象のみ削除する:

```bash
rm -rf <base>/pr-<N>
```

- 対象ごとに個別のコマンドとして実行する（確認していない dir を巻き込むワイルドカードは使わない）
- `~/.revu/jobs/` のジョブ簿・ログは消さない（実行履歴として残す）

### 5. 検証と報告

削除後に必ず確認する:

```bash
ls <base>
```

```bash
revu prune --dry-run
```

- 対象の `pr-<N>` が base 配下から消えていること、prune の一覧からも消えていることを確認して報告する
- merged / closed のレビューが他にも残っている場合は、`revu prune` での一括掃除を選択肢として案内する

## 注意事項

- **確認なしの削除は絶対にしない**。曖昧な指示（「古いの消して」等）は対象一覧を提示して特定してから
- 削除は取り消せない。迷いがありそうなら、削除の代わりに `revu prune` の対象になるまで残す選択肢も提示する
- レビューの中身の確認を求められたら `revu:read`、編集なら `revu:edit` に切り替える
