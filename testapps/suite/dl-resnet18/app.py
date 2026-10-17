import torch
import torchvision

# first output marks "imports done"
print("ready: dl-resnet18")

torch.manual_seed(0)
torch.set_num_threads(1)
model = torchvision.models.resnet18(weights=None).eval()
with torch.no_grad():
    out = model(torch.randn(2, 3, 224, 224))
print(f"logits={tuple(out.shape)} argmax={out.argmax(1).tolist()}")
