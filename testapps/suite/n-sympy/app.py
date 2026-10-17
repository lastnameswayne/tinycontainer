import sympy as sp

# first output marks "imports done"
print("ready: n-sympy")

x = sp.symbols("x")
expr = sp.sin(x) ** 3 * sp.exp(2 * x)
res = sp.simplify(sp.integrate(expr, x))
print(f"integral has {len(str(res))} chars; derivative check: {sp.simplify(sp.diff(res, x) - expr) == 0}")
