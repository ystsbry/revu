## [major] CodeBytes が hintPath 空文字を受けたときの挙動が暗黙的

`lexers.Match(hintPath)` は chroma 側で hintPath が空のときどう振る舞うかが
ドキュメント化されておらず、`Analyse` フォールバックに飛ぶか nil 返しかが
バージョン依存になりがちです。明示的に空判定を入れて分岐したほうが堅いです。

### 提案

```suggestion
	var lexer chroma.Lexer
	if hintPath != "" {
		lexer = lexers.Match(hintPath)
	}
	if lexer == nil {
		lexer = lexers.Analyse(string(content))
	}
```
