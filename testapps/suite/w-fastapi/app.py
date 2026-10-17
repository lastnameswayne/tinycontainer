from fastapi import FastAPI
from fastapi.testclient import TestClient
from pydantic import BaseModel

# first output marks "imports done"
print("ready: w-fastapi")

class Item(BaseModel):
    name: str
    price: float

api = FastAPI()

@api.post("/items")
def create(item: Item):
    return {"name": item.name.upper(), "with_tax": round(item.price * 1.25, 2)}

client = TestClient(api)
r = [client.post("/items", json={"name": f"n{i}", "price": i}).json() for i in range(200)]
print(f"responses={len(r)} last={r[-1]}")
