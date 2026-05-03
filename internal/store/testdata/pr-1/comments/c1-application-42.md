## 設計上の懸念

この箇所で `OrderRepository` を直接呼んでいますが、UnitOfWork 経由にするとトランザクション境界が明確になります。

### 提案

```python
with self.uow:
    order = self.uow.orders.get(order_id)
    self.uow.orders.save(order)
    self.uow.commit()
```
