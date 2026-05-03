## N+1 クエリの懸念

`for order in orders` の中で `session.get` を呼ぶと N+1 になります。`selectinload` で eager に読むのが安全です。
