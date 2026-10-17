from fastapi import FastAPI
from fastapi.testclient import TestClient

# first output marks "imports done"
print("ready: v-fastapi-py311")

api = FastAPI()

@api.get("/ping/{n}")
def ping(n: int):
    return {"pong": n}

client = TestClient(api)
print(f"last={[client.get(f'/ping/{i}').json() for i in range(200)][-1]}")
