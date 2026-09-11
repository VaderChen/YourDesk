#!/usr/bin/env python3
"""將比較報告中的官方 QuickSRNet Small／SESR M5 2× checkpoint 轉成 Core ML。"""
import collections
import sys
from pathlib import Path
import torch
import coremltools as ct
ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "scripts"
sys.path.insert(0, str(SOURCE))
from sr_models.models import QuickSRNetSmall, SESRRelease_M5

class ImageOutput(torch.nn.Module):
    def __init__(self, model):
        super().__init__()
        self.model = model
    def forward(self, image):
        return self.model(image) * 255.0

kind = sys.argv[1] if len(sys.argv) > 1 else "quick"
if kind not in ("quick", "sesr"):
    raise SystemExit("模型必須為 quick 或 sesr")
name = "QuickSRNetSmall" if kind == "quick" else "SESR_M5"
with torch.serialization.safe_globals([torch.optim.Adam, collections.defaultdict, dict]):
    checkpoint = torch.load(SOURCE / f"sr_models/{kind}.pth", map_location="cpu", weights_only=True)
model = QuickSRNetSmall(2) if kind == "quick" else SESRRelease_M5(2)
model.load_state_dict(checkpoint["state_dict"], strict=True)
if kind == "sesr":
    model.collapse()
model.eval()
wrapped = ImageOutput(model).eval()
with torch.no_grad():
    traced = torch.jit.trace(wrapped, torch.zeros(1, 3, 512, 512))
converted = ct.convert(
    traced, convert_to="mlprogram", minimum_deployment_target=ct.target.macOS12,
    compute_precision=ct.precision.FLOAT16,
    inputs=[ct.ImageType(name="input_image", shape=(1, 3, 512, 512), scale=1/255.0, color_layout=ct.colorlayout.RGB)],
    outputs=[ct.ImageType(name="output_image", color_layout=ct.colorlayout.RGB)],
)
converted.short_description = f"{name} 2× · Qualcomm · RGB · FP16"
converted.author = "Qualcomm Innovation Center, Inc."
converted.license = "BSD-3-Clause; see MODEL_LICENSE.txt"
converted.save(str(ROOT / f"internal/superres/{name}_2x.mlpackage"))
