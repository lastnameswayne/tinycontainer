import requests

# first output marks "imports done"
print("ready: w-requests")

s = requests.Session()
reqs = [s.prepare_request(requests.Request("POST", "https://api.example.com/v1/items", json={"i": i},
                                           headers={"X-Trace": str(i)})) for i in range(5000)]
print(f"prepared={len(reqs)} body={reqs[-1].body!r}")
