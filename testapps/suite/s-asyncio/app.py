import asyncio

# first output marks "imports done"
print("ready: s-asyncio")

async def worker(i, q):
    total = 0
    while True:
        item = await q.get()
        if item is None:
            return total
        total += item * i
        await asyncio.sleep(0)

async def main():
    q = asyncio.Queue()
    tasks = [asyncio.create_task(worker(i, q)) for i in range(50)]
    for n in range(100_000):
        await q.put(n % 97)
    for _ in tasks:
        await q.put(None)
    return sum(await asyncio.gather(*tasks))

print(f"total={asyncio.run(main())}")
