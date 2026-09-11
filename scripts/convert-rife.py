#!/usr/bin/env python3
"""RIFE 4.25 Lite：固定尺寸預測光流與遮罩，保留原解析度合成。"""
from pathlib import Path
import torch
import coremltools as ct
import numpy as np
from rife_model.network import IFNet
ROOT=Path(__file__).resolve().parents[1]
net=IFNet().eval()
state=torch.load(ROOT/'scripts/rife_model/flownet.pkl',map_location='cpu',weights_only=True)
state={k.removeprefix('module.'):v for k,v in state.items()}
state={k:v for k,v in state.items() if not k.startswith(("teacher.","caltime."))}
net.load_state_dict(state,strict=True)
class Export(torch.nn.Module):
 def __init__(self):
  super().__init__();self.net=net
 def forward(self,a,b):
  flow,mask,_=self.net(torch.cat((a,b),1),0.5,[32,16,8,4,1])
  return torch.cat((flow[-1],torch.sigmoid(mask)),1)
x=torch.zeros(1,3,384,512)
with torch.no_grad():
 model=Export().eval();model(x,x)
 traced=torch.jit.trace(model,(x,x),check_trace=False)
converted=ct.convert(traced,convert_to='mlprogram',minimum_deployment_target=ct.target.macOS13,
 compute_precision=ct.precision.FLOAT16,
 inputs=[ct.ImageType(name=n,shape=x.shape,scale=1/255.0,color_layout=ct.colorlayout.RGB) for n in ('frame_a','frame_b')],
 outputs=[ct.TensorType(name='flow_mask',dtype=np.float32)])
converted.short_description='RIFE 4.25 Lite midpoint flow and occlusion mask'
converted.license='MIT; see MODEL_LICENSE.txt'
converted.save(str(ROOT/'internal/frameinterp/RIFE425Lite.mlpackage'))
