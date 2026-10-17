import torch

print("ready: torch", flush=True)

torch.manual_seed(0)
a = torch.randn(512, 512)
b = torch.randn(512, 512)
c = a @ b
print(f"matmul: {tuple(c.shape)}, sum={c.sum().item():.3f}")
