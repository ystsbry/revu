---
name: edit
description: 生成済みの revu 形式レビューデータ (~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/ の review.yml + summary.md + comments/*.md) を安全に編集する。「c3 を reject して」「コメントを追加/削除して」「severity を変えて」「レビュー本文を直して」「revu:edit」などと言われたら使う。編集後は必ず revu validate で自検する。
---

# revu:edit

`revu:pr` skill や `revu review` が生成したレビューデータを、revu 形式の取り決めを守って編集する。生成（レビューを書く）は `revu:pr` の仕事。この skill は **既存データの操作** だけを扱う。

## 入力

```
/revu:edit [dir] <編集内容の指示>
```

- `[dir]` (任意): 対象レビューディレクトリ。省略時は cwd のリポジトリから自動解決
- 編集内容の例: 「c3 を reject」「c1 の severity を minor に」「summary の誤字を直して」

## 対象データの構造

```
~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/
├── review.yml          ← メタデータ + コメント参照配列（正）
├── summary.md          ← PR 全体サマリ本文
└── comments/
    ├── c1-{basename}-{line}.md          ← 単一行コメント本文
    ├── c2-{basename}-{start}-{end}.md   ← 範囲コメント本文
    └── ...
```

review.yml が正（source of truth）。comments/*.md は `body_file` で参照される本文。両者の整合が崩れると `revu validate` / `revu open` / `revu submit` が失敗する。

## 手順

### 1. 対象ディレクトリの解決

- ユーザーがパスを指定していればそれを使う
- 指定が無ければ、まず引数なしで `revu validate` を実行する。revu は cwd のリポジトリから最新の PR ディレクトリを自動解決し、出力の `OK <dir> (...)` に絶対パスが出る。それを対象 dir とする
- 複数候補があり曖昧なら `ls ~/.revu/{owner}/{repo}/` で列挙してユーザーに確認する

### 2. 編集前の確認

1. `revu validate <dir>` を実行し、編集前の状態が正常であることと status 件数を控える
2. `<dir>/review.yml` を Read する。**`submitted_at` が存在する場合は既に GitHub へ投稿済み**。編集しても GitHub 側には反映されないので、続行するかユーザーに確認する
3. severity を扱う編集をするなら `revu severities --json` を実行して有効な severity 名の集合を得る。**severity 名をハードコードしない**（`nit`/`minor`/`major`/`critical` はデフォルトにすぎず、設定で差し替わる）

### 3. 編集の実施

操作の種類ごとの取り決めに従う（次節）。ファイル編集は Edit / Write ツールで行う。

### 4. 自検

編集を終えたら必ず実行する:

```bash
revu validate <dir>
```

エラーが出たら出力を読んで修正し、通るまで繰り返す。最後に `OK` 行の status 件数（pending/accepted/rejected/edited）を編集前と比較し、意図どおり変化したことを確認してユーザーに報告する。

## スキーマの取り決め

review.yml のフィールドと制約（`revu validate` が検査する内容）:

| フィールド | 制約 |
|---|---|
| `schema_version` | `1` 固定。変更禁止 |
| `pr.repo` / `pr.number` | 必須。`number` は正の整数。**編集禁止**（別 PR に付け替えない） |
| `pr.head_sha` / `pr.base_branch` | 編集禁止。SHA が変わったら別 dir に再生成するのが正 |
| `generated_at` / `generated_by` | revu 自身が管理する。手で書き換えない |
| `review_event` | `APPROVE` \| `COMMENT` \| `REQUEST_CHANGES` のみ |
| `summary_file` | 必須。参照先ファイルが存在すること |
| `submitted_at` / `review_id` | 投稿時に revu が記録する。**手で追加・削除・変更しない** |
| `comments[].id` | 必須・重複禁止 |
| `comments[].status` | `pending` \| `accepted` \| `rejected` \| `edited` のみ |
| `comments[].severity` | `revu severities --json` が返す `name` のいずれか |
| `comments[].category` | `bug` \| `design` \| `style` \| `perf` \| `security` \| `test` \| `doc` のみ |
| `comments[].path` | 必須。PR ベースリポジトリのルートからの相対パス |
| `comments[].line` | 正の整数。範囲コメントでは**終了行** |
| `comments[].side` | `RIGHT` \| `LEFT` のみ |
| `comments[].start_line` | 任意。正の整数。同一 side なら `start_line <= line` |
| `comments[].start_side` | 任意だが、あるなら `start_line` も必須。削除行→追加行を跨ぐ範囲は `start_side: LEFT` + `side: RIGHT` |
| `comments[].body_file` | 必須。dir 相対パスで、参照先ファイルが存在すること |

## 操作別の取り決め

### status の変更（accept / reject / pending に戻す）

- `revu submit` が投稿するのは **`accepted` と `edited` のみ**。`pending` と `rejected` はスキップされる。ユーザーの「投稿して」「外して」の意図をこのセマンティクスに写像する
- status を変えたら `review_event` の再判定を検討する（後述）

### コメント本文 (comments/*.md) の編集

- 本文を書き換えたら、その `comments[].status` を `edited` に更新する（reject 済みのものを reject のまま推敲した等、明確な意図がある場合を除く）
- 見出しは `## [{severity}] 見出し本文` 形式を維持し、`{severity}` は review.yml の `severity` と**必ず一致**させる。severity を変えるときは両方を同時に変える
- code suggestion は GitHub の ` ```suggestion ` ブロック形式を維持する

### コメントの追加

1. `id` は既存の最大連番の次（`c7` まであれば `c8`）。**既存 id の振り直しはしない**
2. `body_file` は `comments/c{N}-{basename}-{lines}.md`。`{basename}` は対象ファイル名から拡張子を取り英数字以外を `-` に置換、`{lines}` は単一行なら `{line}`、範囲なら `{start_line}-{line}`
3. **ファイル名の数字と review.yml の行番号は必ず一致**させる
4. `line`（と `start_line`）は **PR の diff に含まれる行**であること。不明なら `revu pr diff <N>` で確認する。diff 外の行は GitHub 投稿時に拒否される
5. `status: pending` で追加し、severity は Step 2 で取得した集合から選ぶ
6. 必要なら summary.md の「改善が必要な点」に `c{N}` への言及を追加する

### コメントの削除

- review.yml のエントリ削除と comments/*.md の削除を**セットで**行う（orphan を残さない）
- 残った id は**詰め直さない**（summary.md が `c1`, `c4` のような id を参照しているため）
- summary.md 内に削除したコメントへの言及があれば取り除く

### 行番号・範囲の変更

- `line` / `start_line` を変えたら **body_file のファイル名も一致するようリネーム**し、review.yml の `body_file` も更新する
- 単一行 → 範囲にするなら `start_line` を追加し、ファイル名を `c{N}-{basename}-{start}-{end}.md` に変える（逆も同様）

### summary.md の編集

- 構造（全体所感・良かった点・改善点）は既存の形を尊重して直す
- 本文中のコメント参照 (`c1` 等) が review.yml に実在する id だけを指すよう保つ

### review_event の再判定

コメントの追加・削除・reject・severity 変更をしたら、投稿対象（`pending`/`accepted`/`edited`）のコメントの severity に紐づく `review_event`（`revu severities --json` の値）のうち**最も強いもの**に更新する。優先度は `REQUEST_CHANGES` > `COMMENT` > `APPROVE`、投稿対象 0 件なら `APPROVE`。ユーザーが明示的に review_event を指定した場合はそれを優先する。

## してはいけないこと

- **TUI (`revu open`) が開いている最中の編集**: revu は status 保存時に review.yml を丸ごと書き戻すため、並行編集は失われる。編集前にユーザーへ TUI を閉じているか確認する
- **YAML コメント（`# ...`）に情報を残す**: revu の書き戻しで消えることがある。伝えたいことは summary.md か本文 md に書く
- **`submitted_at` 付きレビューの黙った編集**: 投稿済みの記録。編集自体が無意味なことが多いので必ずユーザーに確認する
- **enum 値の創作**: status / category / side / review_event は上表の値のみ。severity は `revu severities --json` の集合のみ
- **review.yml と md の片側だけ更新**: severity・行番号・ファイル名・id 参照は常に両側を同時に揃える

## 補助コマンド

| コマンド | 用途 |
|---|---|
| `revu validate [dir]` | スキーマ・参照整合の検査（編集の前後で実行） |
| `revu status [dir]` | status 件数の確認 |
| `revu severities --json` | 有効な severity 集合と review_event 対応の取得 |
| `revu pr diff <N>` | 行番号が diff に含まれるかの確認 |
| `revu export [dir]` | GitHub 投稿ペイロードのプレビュー（投稿はしない） |
| `revu now` | ISO 8601 タイムスタンプの取得 |

いずれも `[dir]` を省略すると cwd のリポジトリから最新 PR の dir を自動解決する。
