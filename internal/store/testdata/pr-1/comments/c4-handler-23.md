## エラーハンドリングの抜け

`OrderNotFound` 以外の例外も `500` に潰されるので、少なくとも `ValidationError` は `400` で返すべきです。
