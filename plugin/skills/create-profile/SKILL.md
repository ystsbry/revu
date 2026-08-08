---
name: create-profile
description: revu のリポジトリプロファイルを対話で作成する。リポジトリ登録（default）を最新化し、プロファイル名と対象リポジトリをユーザーに確認して config に [[profile]] を書き込み、validation まで行う。「プロファイルを作って」「リポジトリをグループ分けしたい」「revu:create-profile」などと言われたら使う。
---

# revu:create-profile

登録リポジトリのプロファイル（名前付きサブセット）を、対話で安全に作成する。

`default` = 登録済み全件。この skill は「全件登録の最新化 → プロファイル名の確認 → 対象リポジトリの選択 → config への書き込み → validation」を一連の流れで行う。

## 入力

```
/revu:create-profile
/revu:create-profile <プロファイル名>
```

- `<プロファイル名>` (任意): 先に名前が決まっているとき。省略時は Step 2 で確認する

## 前提

- `revu` が `$PATH` にあること（無ければインストール案内をして終了）
- プロファイル定義は user config（`revu config` の Sources 先頭に出る `config.toml`）に書き込む

## 手順

### 1. default（リポジトリ登録）の作成 or 最新化

まず走査ルートを決める。**それぞれ単独のコマンドとして実行すること**（複合コマンドにしない）:

```bash
ghq root
```

- 成功したらその出力を走査ルートの既定値とする。失敗したら（ghq 不使用）、ユーザーに clone 群のルートディレクトリを尋ねる
- 決めたルートをユーザーに提示して確認してから実行する:

```bash
revu repo scan <root>
```

- 出力の `add:` / `skip (registered):` / `skip (no origin):` を要約して報告する（新規 N 件・登録済み M 件など）
- 検出 0 件かつ登録済みも 0 件なら、ルートが正しいか確認し直す

### 2. プロファイル名の確認

ユーザーにプロファイル名を確認する（引数で与えられていれば追認のみ）。制約:

- `default` は予約名で使用不可
- 既存プロファイルと同名の場合は「更新（repos を置き換え）」の意図か確認する。既存名の一覧は:

```bash
revu profile list
```

- TOML の文字列としてそのまま書ける名前（英数字・ハイフン推奨）を促す

### 3. 対象リポジトリの選択

登録済みリポジトリを取得して番号付きで提示する:

```bash
revu repo list --all
```

- ユーザーには「番号（例: 1,3,5）・slug・owner 名でのまとめ指定（例: acme/* 全部）」いずれでも選ばせる
- 解釈した結果の slug 一覧を提示し、**書き込む前に必ず確認を取る**
- 0 件選択は作成中止として扱う

### 4. config への書き込み

書き込み先は user config。パスは以下の出力の Sources 先頭（lowest priority）:

```bash
revu config
```

ファイルを `Read` してから編集する:

- **新規プロファイル**: ファイル末尾に空行 1 つを挟んで追記する:

```toml
[[profile]]
name = "<プロファイル名>"
repos = [
  "owner/repo-a",
  "owner/repo-b",
]
```

- **既存プロファイルの更新**（Step 2 で確認済みの場合）: 該当 `[[profile]]` ブロック（`[[profile]]` 行から次のテーブルヘッダ行の手前まで）だけを置き換える。**他の行には一切触れない**（コメント・他の設定はそのまま）
- config ファイルが存在しない場合は、ヘッダコメント 1 行 + `[[profile]]` ブロックで新規作成する

### 5. validation チェック

書き込み後、必ず両方を実行して検証する:

```bash
revu config
```

```bash
revu profile list
```

- `revu config` がエラーなく Sources と設定を表示すること（TOML として壊れていないことの確認）
- `revu profile list` に新プロファイルが**選択した件数どおり**（`<name> (N repos)`）表示されること
- `[unregistered: ...]` が表示された場合は slug の typo。Step 3 の選択と突き合わせて修正し、再検証する
- エラーが出た場合は編集内容を見直して修正し、通るまで繰り返す

### 6. 仕上げ

作成結果を報告し、有効化するかユーザーに確認する:

```bash
revu profile use <プロファイル名>
```

- 有効化した場合: 以後 `revu repo list` やダッシュボードはこのプロファイルの範囲で表示されること、`revu profile use default` で全件に戻せることを伝える
- 有効化しない場合: `revu profile use <name>` でいつでも切り替えられることを伝える

## 注意事項

- **config の手編集は `[[profile]]` ブロックだけに限定する**。`[[repo]]` の追加・変更は `revu repo scan/add` に任せる（Step 1 以外で registry を直接書き換えない）
- `active_profile` キーも手で書かない（`revu profile use` が管理する）
- ユーザー確認なしに書き込まない（Step 3 の選択確認と Step 2 の更新確認が必須）
