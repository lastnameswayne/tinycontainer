import torch
from torch import nn

# first output marks "imports done"
print("ready: dl-torch-mlp")

torch.manual_seed(0)
torch.set_num_threads(1)
model = nn.Sequential(nn.Linear(64, 256), nn.ReLU(), nn.Linear(256, 1))
opt = torch.optim.Adam(model.parameters(), lr=1e-3)
X, y = torch.randn(4096, 64), torch.randn(4096, 1)
for _ in range(50):
    opt.zero_grad()
    loss = nn.functional.mse_loss(model(X), y)
    loss.backward()
    opt.step()
print(f"loss={loss.item():.4f}")
