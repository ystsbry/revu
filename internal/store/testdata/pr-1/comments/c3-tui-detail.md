## [minor] preImageContent が同じ path で都度呼ばれる

クロスサイドのコメントを表示するたびに `preImage.Content(path)` が走るため、
キャッシュは PreImageSource 側に任せている前提で問題ないですが、コメント数が
多い PR だと一覧ビュー → 詳細ビュー往復で同じファイルを引き直す形になります。
gitPreImage のキャッシュが効いていれば実質ノーコストですが、観測しておくと安心です。
