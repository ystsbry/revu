# revu コーディングガイドライン（レビュー観点）

revu の revu:pr skill が一般観点に追加で適用する、revu 固有のレビュー指針。

## Go コード

- コメント・識別子は英語。コメントは「コードから読み取れない制約や意図」だけを書く（変更の経緯や次の行の説明は書かない）
- gofmt 必須（CI の format ジョブが `gofmt -l .` で検証する）。lint は golangci-lint v2 の standard セット（errcheck / govet / ineffassign / staticcheck / unused）
- CLI/TUI の出力系では `fmt.Fprint` / `Fprintln` / `Fprintf` の戻り値チェックを免除している（.golangci.yml の errcheck 設定）。それ以外の error は握りつぶさない
- cgo (`import "C"`) は導入しない。リリース・Makefile ともに `CGO_ENABLED=0` 固定で、混入すると ARM ホスト + amd64 Go（issue #1）のような環境でビルドが壊れる。CI の cgo guard は warning に留めるので、レビューで弾く
- リリースターゲット（linux/darwin × amd64/arm64）全てでクロスコンパイルできること。OS/アーキ依存の分岐やパスの決め打ちに注意する
- 子プロセスの実行はシェルを介さず `exec.Command` に引数配列で渡す（インジェクション防止・空白を含む引数の保全）。コードベースに `sh -c` 相当は無いので持ち込まない

## パッケージ構成

- `cmd/revu/` は cobra のコマンド層のみ（1 サブコマンド = 1 `cmd_*.go`）。ロジックは `internal/` に置く
- 主要 internal パッケージ:
  - `internal/store`: `~/.revu/` 配下の探索・読み書き（パスの単一責務化）
  - `internal/config`: config.toml のレイヤリング読み込み
  - `internal/model`: オンディスクの review.yml データモデル
  - `internal/git`: git blob 読み取り（go-git は使わず必要最小限）。`internal/github`: 認証は `gh` CLI に委譲し自前で持たない
  - `internal/claude` / `internal/codex`: レビュー生成ランタイム
  - `internal/render` / `internal/template`: 出力整形
  - `internal/submit`: GitHub へのレビュー投稿（CLI `revu submit` と TUI `:submit` で共通）
  - `internal/diff` / `filter` / `guideline` / `prune`: 差分・フィルタ・ガイドライン読み込み・掃除
  - `internal/tui`: bubbletea の画面
- TUI の実行フローは「TUI が終了して端末を明け渡してから実行する」設計を崩さない（`e` キーのエディタ起動・`:submit` など）

## オンディスクフォーマット / 設定

- レビュー成果物は `~/.revu/{owner}/{repo}/pr-{N}/{sha[:7]}/`（review.yml + summary.md + comments/*.md）。パス生成は `internal/store` に集約し、書き手と読み手で二重定義しない。`REVU_HOME` でルートを差し替えられる（テスト用途）
- 設定は user → `.revu` → `.revu-local` の順で解決・連結される（`~/.config/revu/` < `<repo>/.revu/` < `<repo>/.revu-local/`）。`.revu/` はコミット対象、`.revu-local/` は gitignore 対象という前提を崩さない
- severity・guidelines・templates の相対パスは各 config.toml からの相対で解決される。レイヤーをまたぐ連結挙動を壊さない

## UX / 出力の慣行

- ユーザー向けメッセージには次の行動が分かるヒントを含める
- 進捗・枠などの装飾出力は stderr、データ（review.yml / エクスポート結果など）は stdout（パイプ利用を汚さない）
- 終了コードは子プロセス・失敗ステップのものを伝播する
- 既存の表示スタイル（lipgloss の色番号・TUI の描画慣行）と一貫させる

## テスト

- 新しい挙動には必ずテストを付ける（`t.TempDir` + `REVU_HOME` で実ファイルツリーを作る流儀）
- 失敗系・エッジケース（空・重複・パストラバーサル・レガシー `pr-N/review.yml`・中断）を優先して書く
- TUI は model の Update/View を直接叩くテスト（bubbletea の Program は起動しない）
- CI は `go test ./...` をカバレッジ付きで回し、PR にカバレッジを sticky comment する。カバレッジを下げる変更は理由を添える

## 互換性

- `~/.revu/` のオンディスクフォーマット変更は後方互換（レガシー構成の読み取り継続・自動移行）を必須とする
- CLI のフラグ・サブコマンドの破壊的変更、config.toml のキー変更はコミットメッセージで BREAKING CHANGE を明示する
