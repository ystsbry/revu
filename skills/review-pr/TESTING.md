# review-pr skill 手動スモークテスト

`revu-test-repo` の submodule 化と E2E 自動テストが整うまでの間、`review-pr` skill が壊れていないか確認するための手動チェックリスト。

## 前提

- `gh auth status` が成功する
- `revu` バイナリが `$PATH` にある (`go install ./cmd/revu` 済み)
- skill がインストールされている (`~/.claude/skills/review-pr/` が存在)

## 手順

### 1. 既存の生成物を退避

過去の同 PR レビューがあると差分が分かりにくくなるので退避:

```bash
PR=123  # ← テスト対象の PR 番号に置換
SLUG="$(gh pr view $PR --json baseRepository --jq '.baseRepository.nameWithOwner')"
DIR="$HOME/.revu/$SLUG/pr-$PR"
[ -d "$DIR" ] && mv "$DIR" "$DIR.bak.$(date +%s)"
```

### 2. skill 起動

Claude Code 上で:

```
/review-pr <PR>
/review-pr <PR> --focus security,perf
```

両方試す。

### 3. 出力の確認

```bash
ls -la "$DIR"
ls -la "$DIR/comments"
```

期待:
- `review.yml` が存在
- `summary.md` が存在
- `comments/c{N}-...md` が複数存在（PR の規模に応じて 3〜10 件くらい）

### 4. validate

```bash
revu validate "$DIR"
```

`OK ...` が出ること。エラーが出たら skill のプロンプトに問題あり。

### 5. status

```bash
revu status "$DIR"
```

期待:
- `Comments: N total`
- `pending: N` (全件 pending)
- `accepted: 0`, `rejected: 0`
- `Submitted: not yet`

### 6. open

```bash
cd /path/to/local/clone-of-the-pr-repo
revu open "$DIR"
```

期待:
- TUI が起動
- 一覧画面でサマリ行と各コメントが見える
- カーソルを `c1` に移して Enter → 詳細画面で左ペインに対象コードがハイライト表示される
- `s` でサマリ画面、サマリ本文が glamour レンダリングされる
- `q` で終了

### 7. dry-run submit

```bash
# accept を 1〜2 件付けてから
revu submit --dry-run "$DIR"
```

期待:
- プレビューに head_sha が表示される（GitHub には接続しない、`gh pr view` だけ）
- `Inline comments: N accepted (M rejected, K pending will be skipped)`
- `(dry-run: not submitted)` で終わる

## チェックポイント

skill の品質を評価する観点:

| チェック項目 | 期待 |
|---|---|
| コメント件数 | 5〜10 件程度（PR サイズによる）。20 件超えていたら冗長 |
| severity 分布 | critical/major のみで全部埋まっていないか / nit ばかりで埋もれていないか |
| 根拠の具体性 | 「良くない」だけで終わっておらず、なぜ問題かが書いてある |
| suggestion ブロック | 改善案が示せる場面で `suggestion` ブロックが使われている |
| review_event の判定 | major 以上があれば REQUEST_CHANGES、無ければ COMMENT |
| 既存コードの慣習 | 周辺コードと矛盾するスタイル提案をしていない |
| diff 外への言及 | PR 範囲外のコードには触れていない |

## 既知のチューニングポイント

skill のプロンプト ([SKILL.md](SKILL.md)) で以下を調整して再試行:

- コメント件数が多すぎる → 「§6 重要な指針」の数値目安を調整
- severity が偏る → 「§6 severity 判定」のルーブリックを精緻化
- 形式が崩れる → テンプレート (`templates/*.md.tmpl`) を見直す

## 既存生成物の復元

何か壊した場合:

```bash
mv "$DIR.bak.<timestamp>" "$DIR"
```
