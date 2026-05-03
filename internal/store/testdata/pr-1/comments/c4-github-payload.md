## [major] StartSide が nil のとき start_side を Side で代用しているが、これは GitHub API の意味論と一致しない可能性

このフォールバックは「呼び出し側が同一 side のつもりで `StartLine` だけ渡してきた」
ケースをカバーするために残してある実装ですが、GitHub の API では `start_side` を
省略するとサーバ側でも `side` と同じ扱いになるため、明示的に同じ値を送る必要は
ありません。明示送出が変な挙動を引き起こす将来変更があった場合に備え、`StartSide`
が nil のときは payload からも省略するほうが API のコントラクトに忠実です。
