import jax
import jax.numpy as jnp

# first output marks "imports done"
print("ready: dl-jax")

@jax.jit
def step(w, x, y):
    loss = lambda w: jnp.mean((x @ w - y) ** 2)
    return w - 0.1 * jax.grad(loss)(w), loss(w)

key = jax.random.PRNGKey(0)
x = jax.random.normal(key, (4096, 32))
y = x @ jnp.arange(32.0)
w = jnp.zeros(32)
for _ in range(100):
    w, loss = step(w, x, y)
print(f"loss={float(loss):.4f}")
