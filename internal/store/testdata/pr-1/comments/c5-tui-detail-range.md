## [minor] codeContent の分岐は switch から取り出して関数化したい

`codeContent` の `switch` は 3 ケースとも「描画位置を決めて render を呼ぶ」
という形になっており、本体の責務 (描画ソースの切り替え) はわかりやすい一方、
クロスサイドのケースだけ `crossSideExcerpt` に切り出されている非対称が
気になりました。

### 提案

3 ケースとも同じ粒度のヘルパに揃えるか、逆にクロスサイドもインラインに
戻して switch を一段だけにするか、どちらかにすると読みやすいです。

```go
switch {
case rightOnly:
    return d.renderWorkingTreeRange(c.Path, startLine, endLine)
case sameSide:
    return d.renderPreImageRange(c.Path, startLine, endLine)
default:
    return d.renderCrossSide(c.Path, startSide, startLine, c.Side, endLine)
}
```
