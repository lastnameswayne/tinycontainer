from flask import Flask, jsonify, request

# first output marks "imports done"
print("ready: w-flask")

api = Flask(__name__)

@api.get("/sum")
def total():
    return jsonify(sum=sum(int(x) for x in request.args.getlist("x")))

with api.test_client() as c:
    r = [c.get(f"/sum?x={i}&x={i + 1}").get_json()["sum"] for i in range(500)]
print(f"responses={len(r)} last={r[-1]}")
