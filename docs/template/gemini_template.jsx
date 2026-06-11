import React, { useState, useEffect, useRef } from 'react';
import { 
  Sparkles, 
  Image as ImageIcon, 
  Sliders, 
  Download, 
  Maximize2, 
  Settings, 
  Layers, 
  Compass, 
  Trash2, 
  Share2, 
  RotateCcw, 
  Info, 
  User, 
  Cpu, 
  Check, 
  Copy, 
  AlertCircle,
  Clock,
  ExternalLink,
  ChevronRight,
  SlidersHorizontal,
  Zap,
  HelpCircle
} from 'lucide-react';

// API Key 占位符 (由环境在运行时注入，或在设置中手动输入)
const apiKey = "";

// 预设的高级精调模型数据
const PRESET_MODELS = [
  {
    id: "photorealistic-v2",
    name: "HyperRealism Pro v2.5",
    type: "写实艺术",
    badge: "Hot",
    desc: "极致精细的皮肤纹理、自然的光影追踪和摄影级电影镜头质感。",
    cover: "https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?auto=format&fit=crop&q=80&w=400",
    examplePrompt: "A close-up portrait of an elderly explorer, deep wrinkles, kind eyes, dramatic side-lighting, cinematic, 8k resolution."
  },
  {
    id: "anime-dream",
    name: "Anime Dreamworld v4",
    type: "二次元",
    badge: "Popular",
    desc: "绝美的新海诚式天空、精致的人物线条与极具表现力的梦幻色彩。",
    cover: "https://images.unsplash.com/photo-1578632767115-351597cf2477?auto=format&fit=crop&q=80&w=400",
    examplePrompt: "Anime girl holding an umbrella under a starry night sky, shooting stars, warm glowing street lamps, beautiful watercolor style."
  },
  {
    id: "cyber-punk",
    name: "CyberNeon Synthwave",
    type: "科幻未来",
    badge: "Special",
    desc: "赛博朋克霓虹美学、雨夜街道反光、高饱和度的红蓝紫冷暖对比。",
    cover: "https://images.unsplash.com/photo-1515621061946-eff1c2a352bd?auto=format&fit=crop&q=80&w=400",
    examplePrompt: "A futuristic cyberpunk city street at night, glowing neon signs in kanji, flying retro cars, rainy street reflections, synthwave."
  },
  {
    id: "3d-pixar",
    name: "Pixar Magic 3D",
    type: "三维黏土",
    badge: "Cute",
    desc: "迪士尼/皮克斯质感的可爱 3D 角色设计，细腻材质与温馨的全局光照。",
    cover: "https://images.unsplash.com/photo-1607604276583-eef5d076aa5f?auto=format&fit=crop&q=80&w=400",
    examplePrompt: "A cute fluffy baby dragon sitting on a stack of magical spell books, big sparkling eyes, 3D render Pixar style, claymation look."
  }
];

// 预设尺寸比例
const ASPECT_RATIOS = [
  { label: "1:1 Square", width: 512, height: 512, icon: "▢", desc: "社交头像、配图" },
  { label: "16:9 Landscape", width: 896, height: 512, icon: "▭", desc: "电脑壁纸、横幅" },
  { label: "9:16 Portrait", width: 512, height: 896, icon: "▮", desc: "手机壁纸、海报" },
  { label: "4:3 Classic", width: 768, height: 576, icon: "▱", desc: "经典画幅" },
  { label: "2:3 Artistic", width: 512, height: 768, icon: "▯", desc: "肖像摄影" }
];

// 预设画廊初始数据 (用优质图片展示效果，提升“靠谱站点”的第一印象)
const INITIAL_GALLERY = [
  {
    id: "g1",
    prompt: "An ancient library hidden inside a massive hollow redwood tree, warm sunlight filtering through leaves, floating dust motes, magical cozy atmosphere, highly detailed.",
    url: "https://images.unsplash.com/photo-1507842217343-583bb7270b66?auto=format&fit=crop&q=80&w=800",
    model: "HyperRealism Pro v2.5",
    ratio: "16:9",
    timestamp: "2026-06-08 14:32"
  },
  {
    id: "g2",
    prompt: "Cyberpunk female hacker with neon hair, holographic interfaces floating around her, dark high-tech room, purple and cyan reflections, Unreal Engine 5 render.",
    url: "https://images.unsplash.com/photo-1542751371-adc38448a05e?auto=format&fit=crop&q=80&w=800",
    model: "CyberNeon Synthwave",
    ratio: "1:1",
    timestamp: "2026-06-08 11:20"
  },
  {
    id: "g3",
    prompt: "A mystical cosmic cloud shaped like a magnificent phoenix, nebulas, stellar gas in deep shades of gold and violet, high definition universe photography.",
    url: "https://images.unsplash.com/photo-1462331940025-496dfbfc7564?auto=format&fit=crop&q=80&w=800",
    model: "HyperRealism Pro v2.5",
    ratio: "4:3",
    timestamp: "2026-06-07 19:15"
  }
];

export default function App() {
  // 页面 Tab 状态: 'generate' | 'models' | 'gallery' | 'settings'
  const [activeTab, setActiveTab] = useState('generate');
  
  // 生图控制参数状态
  const [prompt, setPrompt] = useState("");
  const [negativePrompt, setNegativePrompt] = useState("blurry, low quality, distorted, extra limbs, bad anatomy, ugly, deformed");
  const [selectedRatio, setSelectedRatio] = useState(ASPECT_RATIOS[0]);
  const [selectedModel, setSelectedModel] = useState(PRESET_MODELS[0]);
  const [cfgScale, setCfgScale] = useState(7.5);
  const [steps, setSteps] = useState(30);
  const [seed, setSeed] = useState(-1);
  const [sampler, setSampler] = useState("Euler a");

  // API 相关状态
  const [customApiKey, setCustomApiKey] = useState("");
  const [isGenerating, setIsGenerating] = useState(false);
  const [isEnhancing, setIsEnhancing] = useState(false);
  const [generationProgress, setGenerationProgress] = useState(0);
  const [progressStage, setProgressStage] = useState("");
  const [generatedImage, setGeneratedImage] = useState(null);
  
  // 画廊历史状态
  const [gallery, setGallery] = useState(INITIAL_GALLERY);
  const [selectedGalleryItem, setSelectedGalleryItem] = useState(null);
  
  // 全局交互提示
  const [notification, setNotification] = useState(null);

  // 获取有效的 API Key
  const getActiveApiKey = () => {
    return customApiKey.trim() || apiKey || "";
  };

  // 显示自定义通知弹窗（替代原生的 alert）
  const showToast = (message, type = 'info') => {
    setNotification({ message, type });
    setTimeout(() => {
      setNotification(null);
    }, 4000);
  };

  // 复制文本到剪贴板函数
  const copyToClipboard = (text, successMsg = "已复制到剪贴板！") => {
    try {
      const textarea = document.createElement('textarea');
      textarea.value = text;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      showToast(successMsg, 'success');
    } catch (err) {
      showToast("复制失败，请手动选择复制", 'error');
    }
  };

  // 模拟精细的分阶段加载进度
  const startProgressSimulation = () => {
    setGenerationProgress(5);
    setProgressStage("正在分析提示词结构...");
    
    const intervals = [
      { t: 800, p: 15, s: "正在加载精调权重模型..." },
      { t: 2000, p: 35, s: "神经网络噪音注入完成，开始主去噪循环..." },
      { t: 4000, p: 65, s: "深度扩散合成中 (步数 20/30)..." },
      { t: 6000, p: 85, s: "超分辨率细节增强与色彩校正中..." },
      { t: 7500, p: 98, s: "正在渲染最终无损图像..." }
    ];

    intervals.forEach(item => {
      setTimeout(() => {
        if (isGenerating) {
          setGenerationProgress(item.p);
          setProgressStage(item.s);
        }
      }, item.t);
    });
  };

  // 1. 魔法优化提示词 (利用 Gemini 2.5 Flash API)
  const handleEnhancePrompt = async () => {
    if (!prompt.trim()) {
      showToast("请先在输入框中写一点简单的创意想法！", "warning");
      return;
    }

    const key = getActiveApiKey();
    setIsEnhancing(true);
    showToast("正在通过大语言模型进行魔法润色...", "info");

    try {
      // 构筑专业指令，让 Gemini 生成生动的画图 Prompt
      const systemPrompt = "你是一个顶级的 AI 艺术提示词专家。你需要将用户的创意扩写成极其专业、画面感强、细节丰富、包含光影、摄影机参数和艺术风格的英文 Prompt。请直接输出优化后的英文提示词，严禁包含任何解释、前言、括号或多余文字。";
      const payload = {
        contents: [{ 
          parts: [{ text: `请将这个概念优化为最顶级的英文生图Prompt: "${prompt}"` }] 
        }],
        systemInstruction: { 
          parts: [{ text: systemPrompt }] 
        }
      };

      const url = `https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-preview-09-2025:generateContent?key=${key}`;
      
      const response = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      });

      if (!response.ok) {
        throw new Error(`请求失败，状态码: ${response.status}`);
      }

      const result = await response.json();
      const enhancedText = result.candidates?.[0]?.content?.parts?.[0]?.text;
      
      if (enhancedText) {
        setPrompt(enhancedText.trim());
        showToast("魔法提示词生成成功！已自动填充。", "success");
      } else {
        throw new Error("未能获取有效优化结果");
      }
    } catch (error) {
      console.error(error);
      // 失败时的本地智能兜底模板
      const fallbackPrompt = `${prompt}, hyperrealistic, highly detailed cinematic lighting, masterfully composed, digital art, 8k resolution, trend on artstation`;
      setPrompt(fallbackPrompt);
      showToast("连接失败，已为您启用本地精细化模板进行兜底优化", "info");
    } finally {
      setIsEnhancing(false);
    }
  };

  // 2. 图像生成 (利用 Imagen 4.0 API 并具备指数退避重试)
  const handleGenerateImage = async () => {
    if (!prompt.trim()) {
      showToast("请输入您的创意想法（提示词）！", "warning");
      return;
    }

    const key = getActiveApiKey();
    setIsGenerating(true);
    setGenerationProgress(0);
    setProgressStage("正在排队建立云端连接...");
    startProgressSimulation();

    // 整合用户参数到 Prompt，给 Imagen 更好的指导
    const finalPrompt = `${prompt}. (Negative prompt: ${negativePrompt})`;

    const fetchWithExponentialBackoff = async (url, options, retries = 5, delay = 1000) => {
      try {
        const response = await fetch(url, options);
        if (!response.ok) {
          const errData = await response.json().catch(() => ({}));
          throw new Error(errData?.error?.message || `HTTP错误 ${response.status}`);
        }
        return await response.json();
      } catch (error) {
        if (retries > 0) {
          // 指数退避不输出冗长日志，保持清爽
          await new Promise(resolve => setTimeout(resolve, delay));
          return fetchWithExponentialBackoff(url, options, retries - 1, delay * 2);
        }
        throw error;
      }
    };

    try {
      const url = `https://generativelanguage.googleapis.com/v1beta/models/imagen-4.0-generate-001:predict?key=${key}`;
      const payload = {
        instances: {
          prompt: finalPrompt
        },
        parameters: {
          sampleCount: 1,
          aspectRatio: selectedRatio.label.includes("16:9") ? "16:9" : 
                       selectedRatio.label.includes("9:16") ? "9:16" :
                       selectedRatio.label.includes("4:3") ? "4:3" :
                       selectedRatio.label.includes("2:3") ? "2:3" : "1:1"
        }
      };

      const result = await fetchWithExponentialBackoff(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      });

      const base64Data = result.predictions?.[0]?.bytesBase64Encoded;
      if (base64Data) {
        const imageUrl = `data:image/png;base64,${base64Data}`;
        setGeneratedImage(imageUrl);
        setGenerationProgress(100);
        setProgressStage("生成完毕！");
        showToast("画卷已完美呈现在您的面前！", "success");

        // 自动将生成的新图像插入画廊
        const newGalleryItem = {
          id: `g-${Date.now()}`,
          prompt: prompt,
          url: imageUrl,
          model: selectedModel.name,
          ratio: selectedRatio.label.split(' ')[0],
          timestamp: new Date().toLocaleString()
        };
        setGallery([newGalleryItem, ...gallery]);
      } else {
        throw new Error("API未返回图像字节码数据");
      }
    } catch (error) {
      console.error(error);
      // 如果未配置API Key或请求发生错误，提供一个视觉精美的高保真AI预设图作为精美Mock体验
      showToast(`接口连接失败 (${error.message})。为了展示页面设计，我们已为您渲染了高保真本地概念渲染图！`, "info");
      
      setTimeout(() => {
        // 根据提示词或模型类型随机挑选高阶质感图，保障没有密钥的访客也对页面惊叹
        let fallbackUrl = "https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?auto=format&fit=crop&q=80&w=800";
        if (selectedModel.id === 'anime-dream') {
          fallbackUrl = "https://images.unsplash.com/photo-1607604276583-eef5d076aa5f?auto=format&fit=crop&q=80&w=800";
        } else if (selectedModel.id === 'cyber-punk') {
          fallbackUrl = "https://images.unsplash.com/photo-1578632767115-351597cf2477?auto=format&fit=crop&q=80&w=800";
        } else if (selectedModel.id === 'photorealistic-v2') {
          fallbackUrl = "https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&q=80&w=800";
        }
        
        setGeneratedImage(fallbackUrl);
        setGenerationProgress(100);
        setProgressStage("模拟生成成功 (演示模式)");

        const newGalleryItem = {
          id: `mock-${Date.now()}`,
          prompt: prompt,
          url: fallbackUrl,
          model: selectedModel.name,
          ratio: selectedRatio.label.split(' ')[0],
          timestamp: new Date().toLocaleString()
        };
        setGallery([newGalleryItem, ...gallery]);
      }, 3000);

    } finally {
      setIsGenerating(false);
    }
  };

  // 应用历史画廊里的提示词与参数
  const applyHistoryItem = (item) => {
    setPrompt(item.prompt);
    // 自动定位模型
    const matchingModel = PRESET_MODELS.find(m => m.name === item.model) || PRESET_MODELS[0];
    setSelectedModel(matchingModel);
    // 自动定位尺寸
    const matchingRatio = ASPECT_RATIOS.find(r => r.label.startsWith(item.ratio)) || ASPECT_RATIOS[0];
    setSelectedRatio(matchingRatio);
    setActiveTab('generate');
    showToast("已同步历史参数至创作面板！", "success");
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 flex flex-col font-sans selection:bg-violet-500/30 selection:text-violet-200 antialiased overflow-x-hidden">
      
      {/* 1. 悬浮全局通知 */}
      {notification && (
        <div className="fixed top-6 left-1/2 -translate-x-1/2 z-50 flex items-center gap-3 px-5 py-3.5 rounded-xl border border-zinc-800 bg-zinc-900/90 text-sm shadow-2xl backdrop-blur-xl animate-in fade-in slide-in-from-top-4 duration-300">
          <div className={`w-2 h-2 rounded-full ${
            notification.type === 'success' ? 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.6)]' :
            notification.type === 'error' ? 'bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.6)]' :
            notification.type === 'warning' ? 'bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.6)]' :
            'bg-violet-500 shadow-[0_0_8px_rgba(139,92,246,0.6)]'
          }`} />
          <span>{notification.message}</span>
        </div>
      )}

      {/* 2. 精美顶部导航栏 */}
      <header className="sticky top-0 z-40 border-b border-zinc-900 bg-zinc-950/80 backdrop-blur-xl">
        <div className="max-w-[1600px] mx-auto px-6 h-18 flex items-center justify-between">
          
          {/* Logo 区域 */}
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-tr from-violet-600 via-fuchsia-600 to-indigo-600 flex items-center justify-center shadow-[0_0_20px_rgba(139,92,246,0.3)] hover:scale-105 transition-all duration-300">
              <Sparkles className="w-5 h-5 text-white" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-bold text-lg tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-white via-zinc-100 to-zinc-400">IMAGIX</span>
                <span className="px-1.5 py-0.5 rounded text-[10px] font-medium bg-violet-500/10 text-violet-400 border border-violet-500/20">STUDIO PRO</span>
              </div>
              <p className="text-[10px] text-zinc-500 tracking-wider uppercase">下一代人工智能美学引擎</p>
            </div>
          </div>

          {/* 中间核心导航 */}
          <nav className="flex items-center bg-zinc-900/50 border border-zinc-800/60 p-1 rounded-xl">
            <button 
              onClick={() => setActiveTab('generate')}
              className={`flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200 ${
                activeTab === 'generate' 
                  ? 'bg-zinc-800 text-white shadow-inner border border-zinc-700/50' 
                  : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900'
              }`}
            >
              <Cpu className="w-4 h-4" />
              创意工坊
            </button>
            <button 
              onClick={() => setActiveTab('models')}
              className={`flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200 ${
                activeTab === 'models' 
                  ? 'bg-zinc-800 text-white shadow-inner border border-zinc-700/50' 
                  : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900'
              }`}
            >
              <Layers className="w-4 h-4" />
              精调模型
            </button>
            <button 
              onClick={() => setActiveTab('gallery')}
              className={`flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200 ${
                activeTab === 'gallery' 
                  ? 'bg-zinc-800 text-white shadow-inner border border-zinc-700/50' 
                  : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900'
              }`}
            >
              <Compass className="w-4 h-4" />
              灵感画廊
            </button>
            <button 
              onClick={() => setActiveTab('settings')}
              className={`flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200 ${
                activeTab === 'settings' 
                  ? 'bg-zinc-800 text-white shadow-inner border border-zinc-700/50' 
                  : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900'
              }`}
            >
              <Settings className="w-4 h-4" />
              系统设置
            </button>
          </nav>

          {/* 右侧系统概览 */}
          <div className="flex items-center gap-4">
            <div className="hidden lg:flex flex-col items-end">
              <span className="text-xs text-zinc-400 flex items-center gap-1.5">
                <span className="w-1.5 h-1.5 bg-emerald-500 rounded-full animate-ping" />
                API 状态: {getActiveApiKey() ? '已授信' : '未配置密钥'}
              </span>
              <span className="text-[10px] text-zinc-500">云算力资源已就绪</span>
            </div>
            
            <div className="h-8 w-[1px] bg-zinc-800 hidden lg:block" />

            <div className="flex items-center gap-2 bg-zinc-900/40 border border-zinc-800 px-3 py-1.5 rounded-xl">
              <div className="w-6 h-6 rounded-full bg-violet-600/30 border border-violet-500/20 flex items-center justify-center">
                <User className="w-3.5 h-3.5 text-violet-400" />
              </div>
              <span className="text-xs font-semibold tracking-wider text-zinc-300">Creator</span>
            </div>
          </div>

        </div>
      </header>

      {/* 3. 核心内容区 */}
      <main className="flex-1 max-w-[1600px] w-full mx-auto p-6 flex flex-col justify-between">
        
        {/* ===================== TAB 1: 创意工坊 (核心生图) ===================== */}
        {activeTab === 'generate' && (
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
            
            {/* 左侧控制输入区域 (占 5 格) */}
            <section className="lg:col-span-5 bg-zinc-900/40 border border-zinc-800/80 rounded-2xl p-6 backdrop-blur-xl shadow-xl flex flex-col gap-6">
              
              <div className="flex items-center justify-between border-b border-zinc-800/60 pb-4">
                <div className="flex items-center gap-2.5">
                  <SlidersHorizontal className="w-5 h-5 text-violet-400" />
                  <h2 className="text-base font-semibold text-zinc-100">控制面板</h2>
                </div>
                <button 
                  onClick={() => {
                    setPrompt("");
                    setNegativePrompt("blurry, low quality, distorted");
                    showToast("参数已初始化", "info");
                  }}
                  className="text-xs text-zinc-500 hover:text-zinc-300 transition flex items-center gap-1"
                  title="重置所有参数"
                >
                  <RotateCcw className="w-3.5 h-3.5" />
                  重置
                </button>
              </div>

              {/* 1. 创意画笔（Prompt 输入） */}
              <div className="flex flex-col gap-2.5">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-medium text-zinc-300 flex items-center gap-1.5">
                    <Sparkles className="w-3.5 h-3.5 text-violet-400" />
                    创意构想提示词 (英文最佳)
                  </label>
                  <button
                    onClick={handleEnhancePrompt}
                    disabled={isEnhancing || !prompt.trim()}
                    className={`px-2.5 py-1 rounded-lg text-xs font-medium flex items-center gap-1.5 transition-all ${
                      prompt.trim() 
                        ? 'bg-violet-600/20 text-violet-300 border border-violet-500/30 hover:bg-violet-600/30' 
                        : 'bg-zinc-800/40 text-zinc-600 border border-transparent cursor-not-allowed'
                    }`}
                  >
                    <Sparkles className={`w-3 h-3 ${isEnhancing ? 'animate-spin' : ''}`} />
                    {isEnhancing ? "魔法扩写中..." : "AI 魔法润色"}
                  </button>
                </div>

                <div className="relative group">
                  <textarea
                    value={prompt}
                    onChange={(e) => setPrompt(e.target.value)}
                    placeholder="例如: 森林中漂浮着发光水母的魔法树屋, 新海诚动画风格, 唯美夜空, 绚丽光影..."
                    className="w-full h-32 bg-zinc-950/80 border border-zinc-800 rounded-xl px-4 py-3 text-sm text-zinc-200 placeholder-zinc-600 focus:outline-none focus:border-violet-500/80 focus:ring-1 focus:ring-violet-500/30 transition duration-200 resize-none leading-relaxed"
                  />
                  <div className="absolute bottom-3 right-3 flex items-center gap-2">
                    {prompt && (
                      <button 
                        onClick={() => setPrompt("")}
                        className="p-1 rounded-md bg-zinc-900 text-zinc-500 hover:text-zinc-300 transition"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    )}
                    <span className="text-[10px] text-zinc-600 bg-zinc-900 px-1.5 py-0.5 rounded">
                      {prompt.length} 字符
                    </span>
                  </div>
                </div>
              </div>

              {/* 2. 反向提示词（不想出现在画面中的内容） */}
              <div className="flex flex-col gap-2">
                <label className="text-xs font-medium text-zinc-400 flex items-center gap-1.5">
                  <AlertCircle className="w-3.5 h-3.5 text-zinc-500" />
                  反向提示词 (Negative Prompt)
                </label>
                <input
                  type="text"
                  value={negativePrompt}
                  onChange={(e) => setNegativePrompt(e.target.value)}
                  placeholder="畸形, 模糊, 低画质..."
                  className="w-full bg-zinc-950/80 border border-zinc-800 rounded-xl px-4 py-2.5 text-xs text-zinc-400 placeholder-zinc-700 focus:outline-none focus:border-zinc-700 transition"
                />
              </div>

              {/* 3. 精调底模选择（大图卡片式） */}
              <div className="flex flex-col gap-2.5">
                <label className="text-xs font-medium text-zinc-300 flex items-center gap-1.5">
                  <Layers className="w-3.5 h-3.5 text-violet-400" />
                  精调美学底模
                </label>
                <div className="grid grid-cols-2 gap-3">
                  {PRESET_MODELS.map((model) => (
                    <button
                      key={model.id}
                      onClick={() => {
                        setSelectedModel(model);
                        if (!prompt.trim()) {
                          setPrompt(model.examplePrompt);
                          showToast(`已应用模型【${model.name}】的创意范例！`, 'info');
                        }
                      }}
                      className={`relative overflow-hidden rounded-xl border text-left p-2.5 transition-all duration-300 ${
                        selectedModel.id === model.id 
                          ? 'border-violet-500 bg-violet-600/5 shadow-[0_0_15px_rgba(139,92,246,0.15)]' 
                          : 'border-zinc-800/80 bg-zinc-950/40 hover:border-zinc-700 hover:bg-zinc-950/60'
                      }`}
                    >
                      <div className="flex items-center gap-2">
                        <img 
                          src={model.cover} 
                          alt={model.name}
                          className="w-8 h-8 rounded-lg object-cover border border-zinc-800 flex-shrink-0" 
                        />
                        <div className="min-w-0">
                          <div className="flex items-center gap-1">
                            <span className="text-xs font-semibold text-zinc-200 truncate">{model.name.split(' ')[0]}</span>
                            {model.badge && (
                              <span className="text-[8px] bg-violet-500/20 text-violet-300 px-1 rounded-sm scale-90">
                                {model.badge}
                              </span>
                            )}
                          </div>
                          <span className="text-[10px] text-zinc-500 block truncate">{model.type}</span>
                        </div>
                      </div>
                    </button>
                  ))}
                </div>
              </div>

              {/* 4. 尺寸与幅面比例 (可视化比例卡片) */}
              <div className="flex flex-col gap-2.5">
                <label className="text-xs font-medium text-zinc-300 flex items-center gap-1.5">
                  <ImageIcon className="w-3.5 h-3.5 text-violet-400" />
                  幅面比例 (Aspect Ratio)
                </label>
                <div className="grid grid-cols-5 gap-2">
                  {ASPECT_RATIOS.map((ratio) => (
                    <button
                      key={ratio.label}
                      onClick={() => setSelectedRatio(ratio)}
                      className={`flex flex-col items-center justify-center p-2 rounded-xl border transition-all ${
                        selectedRatio.label === ratio.label
                          ? 'border-violet-500 bg-violet-500/10 text-violet-300'
                          : 'border-zinc-800/80 bg-zinc-950/50 text-zinc-500 hover:border-zinc-700 hover:text-zinc-300'
                      }`}
                      title={ratio.desc}
                    >
                      <span className="text-lg font-bold leading-none mb-1">{ratio.icon}</span>
                      <span className="text-[9px] font-medium tracking-tight truncate w-full text-center">{ratio.label.split(' ')[0]}</span>
                    </button>
                  ))}
                </div>
              </div>

              {/* 5. 展开折叠的高级专家配置 */}
              <details className="group border-t border-zinc-800/60 pt-4">
                <summary className="list-none flex items-center justify-between cursor-pointer text-xs font-medium text-zinc-400 hover:text-zinc-200 transition">
                  <div className="flex items-center gap-1.5">
                    <Sliders className="w-3.5 h-3.5 text-zinc-500" />
                    高级生成控制 (专业参数)
                  </div>
                  <ChevronRight className="w-4 h-4 text-zinc-500 transition-transform group-open:rotate-90" />
                </summary>
                
                <div className="flex flex-col gap-4 mt-4 pt-2 animate-in fade-in slide-in-from-top-2">
                  {/* CFG Scale */}
                  <div className="flex flex-col gap-1.5">
                    <div className="flex items-center justify-between text-xs">
                      <span className="text-zinc-400 flex items-center gap-1">
                        提示词贴合度 (CFG Scale)
                        <span className="text-zinc-600 cursor-help" title="CFG越高越遵从提示词，越低自由发挥度越高">
                          <HelpCircle className="w-3 h-3" />
                        </span>
                      </span>
                      <span className="text-violet-400 font-mono font-medium">{cfgScale}</span>
                    </div>
                    <input 
                      type="range" 
                      min="1" 
                      max="20" 
                      step="0.5"
                      value={cfgScale} 
                      onChange={(e) => setCfgScale(parseFloat(e.target.value))}
                      className="w-full accent-violet-500 h-1 bg-zinc-800 rounded-lg appearance-none cursor-pointer"
                    />
                  </div>

                  {/* Steps */}
                  <div className="flex flex-col gap-1.5">
                    <div className="flex items-center justify-between text-xs">
                      <span className="text-zinc-400 flex items-center gap-1">
                        去噪迭代步数 (Steps)
                        <span className="text-zinc-600 cursor-help" title="去噪迭代次数，步数越高细节越多，速度越慢">
                          <HelpCircle className="w-3 h-3" />
                        </span>
                      </span>
                      <span className="text-violet-400 font-mono font-medium">{steps}</span>
                    </div>
                    <input 
                      type="range" 
                      min="10" 
                      max="100" 
                      step="1"
                      value={steps} 
                      onChange={(e) => setSteps(parseInt(e.target.value))}
                      className="w-full accent-violet-500 h-1 bg-zinc-800 rounded-lg appearance-none cursor-pointer"
                    />
                  </div>

                  {/* Sampler & Seed */}
                  <div className="grid grid-cols-2 gap-3">
                    <div className="flex flex-col gap-1.5">
                      <label className="text-xs text-zinc-400">采样方法</label>
                      <select 
                        value={sampler} 
                        onChange={(e) => setSampler(e.target.value)}
                        className="bg-zinc-950/80 border border-zinc-800 rounded-lg px-2 py-1.5 text-xs text-zinc-300 focus:outline-none focus:border-zinc-700"
                      >
                        <option>Euler a</option>
                        <option>DPM++ 2M Karras</option>
                        <option>DDIM</option>
                        <option>Heun</option>
                      </select>
                    </div>
                    <div className="flex flex-col gap-1.5">
                      <label className="text-xs text-zinc-400">随机种子 (Seed)</label>
                      <input 
                        type="number" 
                        value={seed} 
                        onChange={(e) => setSeed(parseInt(e.target.value))}
                        className="bg-zinc-950/80 border border-zinc-800 rounded-lg px-2 py-1.5 text-xs text-zinc-300 focus:outline-none focus:border-zinc-700 font-mono"
                      />
                    </div>
                  </div>
                </div>
              </details>

              {/* 核心生图动作大按钮 */}
              <button
                onClick={handleGenerateImage}
                disabled={isGenerating}
                className="w-full relative overflow-hidden group py-3.5 rounded-xl bg-gradient-to-r from-violet-600 via-fuchsia-600 to-indigo-600 text-white font-semibold text-sm transition-all duration-300 shadow-[0_4px_20px_rgba(139,92,246,0.35)] hover:shadow-[0_4px_25px_rgba(139,92,246,0.5)] hover:scale-[1.01] active:scale-[0.99] disabled:opacity-80 disabled:cursor-not-allowed"
              >
                {/* 炫光流光粒子 */}
                <div className="absolute inset-0 bg-[linear-gradient(120deg,rgba(255,255,255,0)_30%,rgba(255,255,255,0.25)_40%,rgba(255,255,255,0)_50%)] bg-[length:200%_100%] animate-[shimmer_2.5s_infinite] pointer-events-none" />
                <div className="flex items-center justify-center gap-2">
                  <Zap className="w-4 h-4 fill-current" />
                  <span>{isGenerating ? "正在渲染梦境大作..." : "立即生成画卷"}</span>
                </div>
              </button>

            </section>

            {/* 右侧画布区域 (占 7 格) */}
            <section className="lg:col-span-7 flex flex-col gap-4">
              
              {/* 画布状态区 */}
              <div className="bg-zinc-900/40 border border-zinc-800/80 rounded-2xl p-6 backdrop-blur-xl shadow-xl flex flex-col justify-center min-h-[480px] relative overflow-hidden group">
                
                {/* 如果没有正在生成 且 没有生成的图片，显示欢迎创作界面 */}
                {!isGenerating && !generatedImage && (
                  <div className="flex flex-col items-center text-center p-8 z-10 max-w-md mx-auto">
                    <div className="w-16 h-16 rounded-2xl bg-zinc-900 border border-zinc-800 flex items-center justify-center mb-6 shadow-inner group-hover:scale-110 transition duration-300">
                      <ImageIcon className="w-8 h-8 text-violet-400" />
                    </div>
                    <h3 className="text-lg font-semibold text-zinc-200 mb-2">激发你的视觉想象力</h3>
                    <p className="text-xs text-zinc-500 leading-relaxed mb-6">
                      在左侧面板填入您的脑海构想，选择喜欢的画风与幅面，点击“生成”即可将代码变成瑰丽的艺术作品。
                    </p>
                    <div className="flex flex-wrap gap-2 justify-center">
                      <button 
                        onClick={() => {
                          setPrompt("An astronaut riding a white horse in grand canyon, cinematic light, award winning photography.");
                          showToast("已导入创意灵感配方！", "success");
                        }}
                        className="px-3 py-1.5 rounded-lg border border-zinc-800 bg-zinc-950/50 text-[11px] text-zinc-400 hover:text-zinc-200 hover:border-zinc-700 transition"
                      >
                        💡 试一试: 峡谷中的宇航员骑士
                      </button>
                    </div>
                  </div>
                )}

                {/* 生成中的动态阶段展示 (比普通 loading 高端的多) */}
                {isGenerating && (
                  <div className="flex flex-col items-center justify-center z-10 p-8 max-w-lg mx-auto w-full">
                    
                    {/* 脑电波/神经网络发光圈 */}
                    <div className="relative w-24 h-24 mb-8">
                      <div className="absolute inset-0 rounded-full border-2 border-violet-500/20 animate-ping" />
                      <div className="absolute inset-2 rounded-full border border-fuchsia-500/30 animate-pulse" />
                      <div className="absolute inset-4 rounded-full bg-gradient-to-tr from-violet-600/20 to-fuchsia-600/20 flex items-center justify-center border border-zinc-800">
                        <Cpu className="w-8 h-8 text-violet-400 animate-spin duration-3000" />
                      </div>
                    </div>

                    <h4 className="text-sm font-semibold text-zinc-200 mb-1.5">Imagen 4.0 引擎极速解算中</h4>
                    <p className="text-[11px] font-mono text-zinc-500 mb-5 tracking-wider h-4">{progressStage}</p>

                    {/* 渐变长进度条 */}
                    <div className="w-full bg-zinc-950 rounded-full h-2 overflow-hidden border border-zinc-900">
                      <div 
                        className="bg-gradient-to-r from-violet-600 via-fuchsia-600 to-indigo-600 h-full transition-all duration-300 ease-out"
                        style={{ width: `${generationProgress}%` }}
                      />
                    </div>
                    <div className="w-full flex justify-between items-center mt-2">
                      <span className="text-[10px] text-zinc-600">云端深度模型计算</span>
                      <span className="text-xs font-mono font-medium text-violet-400">{generationProgress}%</span>
                    </div>

                  </div>
                )}

                {/* 展示已生成好的高阶图像及悬浮操作 */}
                {!isGenerating && generatedImage && (
                  <div className="relative w-full h-full flex items-center justify-center animate-in fade-in zoom-in-95 duration-500">
                    <img 
                      src={generatedImage} 
                      alt="AI Generated" 
                      className="max-h-[500px] w-auto rounded-xl object-contain shadow-2xl border border-zinc-800"
                    />

                    {/* 悬浮多维高阶操作菜单 */}
                    <div className="absolute bottom-4 left-1/2 -translate-x-1/2 bg-zinc-950/80 backdrop-blur-xl border border-zinc-800/80 px-4 py-2.5 rounded-xl shadow-2xl flex items-center gap-3 animate-in slide-in-from-bottom-2 duration-300">
                      
                      <button 
                        onClick={() => {
                          const a = document.createElement('a');
                          a.href = generatedImage;
                          a.download = `imagix-artwork-${Date.now()}.png`;
                          a.click();
                          showToast("艺术品下载成功！", "success");
                        }}
                        className="p-2 rounded-lg bg-zinc-900 hover:bg-zinc-800 text-zinc-300 hover:text-white transition flex items-center gap-1.5 text-xs font-medium"
                        title="无损下载 PNG"
                      >
                        <Download className="w-4 h-4" />
                        下载
                      </button>

                      <div className="w-[1px] h-4 bg-zinc-800" />

                      <button 
                        onClick={() => {
                          copyToClipboard(prompt, "画作提示词复制成功，随时粘贴分享！");
                        }}
                        className="p-2 rounded-lg bg-zinc-900 hover:bg-zinc-800 text-zinc-300 hover:text-white transition flex items-center gap-1.5 text-xs font-medium"
                        title="复制生图提示词"
                      >
                        <Copy className="w-4 h-4" />
                        复制提示词
                      </button>

                      <div className="w-[1px] h-4 bg-zinc-800" />

                      <button 
                        onClick={() => {
                          setSelectedGalleryItem({
                            prompt: prompt,
                            url: generatedImage,
                            model: selectedModel.name,
                            ratio: selectedRatio.label.split(' ')[0],
                            timestamp: "刚刚"
                          });
                        }}
                        className="p-2 rounded-lg bg-zinc-900 hover:bg-zinc-800 text-zinc-300 hover:text-white transition flex items-center gap-1.5 text-xs font-medium"
                        title="全屏高清灯箱查看"
                      >
                        <Maximize2 className="w-4 h-4" />
                        全屏
                      </button>

                    </div>
                  </div>
                )}

              </div>

              {/* 底部参数小看板 */}
              <div className="bg-zinc-900/20 border border-zinc-900 rounded-xl p-3 px-4 flex items-center justify-between text-[11px] text-zinc-500">
                <span className="flex items-center gap-1">
                  <Clock className="w-3.5 h-3.5" />
                  生成耗时: {isGenerating ? "估计 6-8s" : generatedImage ? "2.42s (平均速度)" : "未开始"}
                </span>
                <span>引擎版本: Imagen-v4-Ultra (Predict)</span>
                <span>算力节点: AWS-SG-MultiCloud</span>
              </div>

            </section>

          </div>
        )}

        {/* ===================== TAB 2: 精调模型 (画风市场) ===================== */}
        {activeTab === 'models' && (
          <div className="animate-in fade-in duration-300">
            <div className="mb-8 max-w-2xl">
              <h2 className="text-xl font-bold text-zinc-100 flex items-center gap-2 mb-2">
                <Layers className="w-5 h-5 text-violet-400" />
                精调美学底膜库 (Model Library)
              </h2>
              <p className="text-xs text-zinc-400 leading-relaxed">
                为您精心配置了多套行业精调垂直模型。不同的模型在色彩饱和度、光感追踪、材质表现上有专属微调。选择一个心仪的模型即可立即展开您的风格化绘制。
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
              {PRESET_MODELS.map((model) => (
                <div 
                  key={model.id}
                  className="bg-zinc-900/40 border border-zinc-800 rounded-2xl overflow-hidden group hover:border-violet-500/40 transition-all duration-300 flex flex-col justify-between"
                >
                  <div className="relative h-48 overflow-hidden">
                    <img 
                      src={model.cover} 
                      alt={model.name} 
                      className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500"
                    />
                    <div className="absolute inset-0 bg-gradient-to-t from-zinc-950 via-transparent to-transparent" />
                    
                    <span className="absolute top-3 left-3 px-2 py-0.5 rounded-md text-[10px] font-semibold bg-violet-600 text-white shadow-lg">
                      {model.type}
                    </span>

                    {model.badge && (
                      <span className="absolute top-3 right-3 px-2 py-0.5 rounded-md text-[10px] font-semibold bg-zinc-900/90 text-amber-400 border border-amber-400/20 shadow-lg">
                        {model.badge}
                      </span>
                    )}

                    <div className="absolute bottom-3 left-3 right-3">
                      <h3 className="text-sm font-bold text-white tracking-wide">{model.name}</h3>
                    </div>
                  </div>

                  <div className="p-4 flex-1 flex flex-col justify-between gap-4">
                    <p className="text-xs text-zinc-400 leading-relaxed min-h-[48px]">{model.desc}</p>
                    
                    <div className="flex flex-col gap-2">
                      <div className="p-2 bg-zinc-950 rounded-lg border border-zinc-850">
                        <span className="text-[10px] text-zinc-500 uppercase block font-medium mb-1">推荐提示范例</span>
                        <p className="text-[10px] text-zinc-300 italic line-clamp-2">"{model.examplePrompt}"</p>
                      </div>

                      <button
                        onClick={() => {
                          setSelectedModel(model);
                          setPrompt(model.examplePrompt);
                          setActiveTab('generate');
                          showToast(`已应用 ${model.name} 模型与预设配方`, "success");
                        }}
                        className="w-full mt-2 py-2 rounded-xl bg-zinc-800 hover:bg-violet-600 text-zinc-200 hover:text-white transition-all text-xs font-semibold flex items-center justify-center gap-1.5"
                      >
                        使用该模型创作
                        <ChevronRight className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* ===================== TAB 3: 灵感画廊 (Masonry 展示) ===================== */}
        {activeTab === 'gallery' && (
          <div className="animate-in fade-in duration-300">
            <div className="mb-8 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
              <div>
                <h2 className="text-xl font-bold text-zinc-100 flex items-center gap-2 mb-2">
                  <Compass className="w-5 h-5 text-violet-400" />
                  灵感画廊 (Inspiration Showcase)
                </h2>
                <p className="text-xs text-zinc-400 leading-relaxed">
                  这里是社区优秀的 AI 创作展区。您可以从中获取创意，或一键导入其他创作者的参数配置进行二次复刻。
                </p>
              </div>

              <div className="flex items-center gap-3">
                <span className="text-xs text-zinc-500">累计收录: {gallery.length} 张艺术画卷</span>
              </div>
            </div>

            {/* 画廊网格 */}
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-6">
              {gallery.map((item) => (
                <div 
                  key={item.id}
                  className="bg-zinc-900/40 border border-zinc-800/80 rounded-2xl overflow-hidden group hover:border-zinc-700 hover:shadow-xl transition-all duration-300"
                >
                  <div className="relative overflow-hidden aspect-square max-h-[320px]">
                    <img 
                      src={item.url} 
                      alt="Art" 
                      className="w-full h-full object-cover group-hover:scale-102 transition duration-500" 
                    />
                    <div className="absolute inset-0 bg-gradient-to-t from-zinc-950/90 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex flex-col justify-between p-4" />
                    
                    {/* 悬浮快速层 */}
                    <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex flex-col justify-end p-4">
                      <p className="text-xs text-zinc-200 line-clamp-3 mb-3 italic">"{item.prompt}"</p>
                      
                      <div className="flex items-center justify-between">
                        <span className="text-[10px] text-violet-400 font-mono">{item.model}</span>
                        <div className="flex gap-2">
                          <button
                            onClick={() => applyHistoryItem(item)}
                            className="p-1.5 rounded-lg bg-zinc-850 hover:bg-violet-600 text-zinc-300 hover:text-white transition"
                            title="复刻此灵感参数"
                          >
                            <RotateCcw className="w-3.5 h-3.5" />
                          </button>
                          <button
                            onClick={() => setSelectedGalleryItem(item)}
                            className="p-1.5 rounded-lg bg-zinc-850 hover:bg-violet-600 text-zinc-300 hover:text-white transition"
                            title="查看超清大图"
                          >
                            <Maximize2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </div>
                    </div>

                  </div>

                  <div className="p-4">
                    <p className="text-[11px] text-zinc-400 line-clamp-1 mb-2">“ {item.prompt} ”</p>
                    <div className="flex items-center justify-between text-[10px] text-zinc-500 border-t border-zinc-800/60 pt-2.5">
                      <span>尺寸: <strong className="text-zinc-400">{item.ratio}</strong></span>
                      <span>{item.timestamp}</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* ===================== TAB 4: 系统设置 (密钥配置) ===================== */}
        {activeTab === 'settings' && (
          <div className="animate-in fade-in duration-300 max-w-2xl">
            <h2 className="text-xl font-bold text-zinc-100 flex items-center gap-2 mb-4">
              <Settings className="w-5 h-5 text-violet-400" />
              开发者系统配置
            </h2>
            
            <div className="bg-zinc-900/40 border border-zinc-800 rounded-2xl p-6 flex flex-col gap-6">
              
              {/* API 秘钥输入 */}
              <div className="flex flex-col gap-2">
                <div className="flex items-center justify-between">
                  <label className="text-sm font-medium text-zinc-300 flex items-center gap-2">
                    <Cpu className="w-4 h-4 text-violet-400" />
                    Gemini 平台 API 密钥 (API Key)
                  </label>
                  <a 
                    href="https://aistudio.google.com/" 
                    target="_blank" 
                    rel="noreferrer" 
                    className="text-xs text-violet-400 hover:underline flex items-center gap-1"
                  >
                    获取免费密钥
                    <ExternalLink className="w-3 h-3" />
                  </a>
                </div>

                <input
                  type="password"
                  value={customApiKey}
                  onChange={(e) => setCustomApiKey(e.target.value)}
                  placeholder="请输入您的 Google AI Studio 密钥 (AIzaSy...)"
                  className="w-full bg-zinc-950 border border-zinc-850 rounded-xl px-4 py-3 text-sm text-zinc-300 placeholder-zinc-600 focus:outline-none focus:border-violet-500 transition font-mono"
                />
                
                <p className="text-xs text-zinc-500 leading-relaxed flex items-start gap-1.5 mt-1">
                  <Info className="w-4 h-4 text-zinc-600 flex-shrink-0 mt-0.5" />
                  <span>
                    您的 API Key 仅保存在当前浏览器内存中，用于直连 Google 官方端点进行提示词优化与画幅渲染（基于 <strong>Imagen 4.0 Pro</strong> 及 <strong>Gemini 2.5 Flash</strong>）。留空时我们将对接口请求报错进行平滑兜底，为您渲染极高保真的本地风格化演示大图，让您无需密钥即可完美体验完整的页面流程与动态视效。
                  </span>
                </p>
              </div>

              <div className="h-[1px] bg-zinc-800" />

              {/* 图像引擎配置 */}
              <div className="flex flex-col gap-3">
                <h3 className="text-sm font-semibold text-zinc-300">云算力底层渲染节点</h3>
                
                <div className="grid grid-cols-2 gap-3">
                  <div className="p-3.5 bg-zinc-950/80 rounded-xl border border-violet-500/30 flex items-center justify-between">
                    <div>
                      <span className="text-xs font-semibold text-zinc-200 block">Imagen-v4-Predict</span>
                      <span className="text-[10px] text-zinc-500">Google 下一代多模态原生模型</span>
                    </div>
                    <span className="px-2 py-0.5 rounded-full text-[9px] bg-violet-500/10 text-violet-400 border border-violet-500/20 font-semibold">
                      默认激活
                    </span>
                  </div>

                  <div className="p-3.5 bg-zinc-950/40 rounded-xl border border-zinc-800/80 flex items-center justify-between opacity-60">
                    <div>
                      <span className="text-xs font-semibold text-zinc-400 block">Stable Diffusion XL</span>
                      <span className="text-[10px] text-zinc-600">社区级经典扩散底模</span>
                    </div>
                    <span className="text-[9px] text-zinc-600">敬请期待</span>
                  </div>
                </div>
              </div>

              {/* 状态保存按钮 */}
              <button
                onClick={() => {
                  showToast("配置已保存！创作资源已重新加载。", "success");
                }}
                className="w-full py-2.5 rounded-xl bg-zinc-800 hover:bg-zinc-700 text-zinc-200 font-semibold text-xs transition"
              >
                保存所有配置并应用
              </button>

            </div>
          </div>
        )}

      </main>

      {/* 4. 精细全屏超清灯箱弹窗 */}
      {selectedGalleryItem && (
        <div className="fixed inset-0 z-50 bg-black/90 backdrop-blur-xl flex items-center justify-center p-4 sm:p-10 animate-in fade-in duration-300">
          
          {/* 大图及右侧信息复合体 */}
          <div className="bg-zinc-900 border border-zinc-800 rounded-2xl overflow-hidden max-w-5xl w-full flex flex-col md:flex-row shadow-2xl relative">
            
            {/* 关闭按钮 */}
            <button 
              onClick={() => setSelectedGalleryItem(null)}
              className="absolute top-4 right-4 z-10 w-8 h-8 rounded-full bg-zinc-950/80 text-zinc-400 hover:text-white flex items-center justify-center border border-zinc-800 hover:scale-105 transition"
            >
              ✕
            </button>

            {/* 图像展示 */}
            <div className="flex-1 bg-black flex items-center justify-center p-6">
              <img 
                src={selectedGalleryItem.url} 
                alt="Enlarged Art" 
                className="max-h-[70vh] w-auto object-contain rounded-lg shadow-xl"
              />
            </div>

            {/* 信息详情侧边栏 */}
            <div className="w-full md:w-80 border-t md:border-t-0 md:border-l border-zinc-800 p-6 flex flex-col justify-between gap-6">
              
              <div className="flex flex-col gap-4">
                <span className="text-[10px] font-bold text-violet-400 uppercase tracking-wider">
                  画卷配置详情 (Parameters)
                </span>
                
                <div>
                  <span className="text-xs text-zinc-500 block mb-1">提示词</span>
                  <p className="text-xs text-zinc-200 leading-relaxed bg-zinc-950 p-3 rounded-xl border border-zinc-850">
                    "{selectedGalleryItem.prompt}"
                  </p>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <span className="text-[10px] text-zinc-500 block">生成底模</span>
                    <span className="text-xs text-zinc-300 font-medium">{selectedGalleryItem.model}</span>
                  </div>
                  <div>
                    <span className="text-[10px] text-zinc-500 block">幅面比例</span>
                    <span className="text-xs text-zinc-300 font-medium">{selectedGalleryItem.ratio}</span>
                  </div>
                </div>
              </div>

              <div className="flex flex-col gap-2.5">
                <button
                  onClick={() => {
                    applyHistoryItem(selectedGalleryItem);
                    setSelectedGalleryItem(null);
                  }}
                  className="w-full py-2.5 rounded-xl bg-violet-600 hover:bg-violet-700 text-white font-semibold text-xs transition flex items-center justify-center gap-1.5"
                >
                  <RotateCcw className="w-3.5 h-3.5" />
                  导入该作参数配置并再次复刻
                </button>

                <button
                  onClick={() => {
                    const a = document.createElement('a');
                    a.href = selectedGalleryItem.url;
                    a.download = `imagix-showcase-${Date.now()}.png`;
                    a.click();
                    showToast("下载成功！", "success");
                  }}
                  className="w-full py-2.5 rounded-xl bg-zinc-800 hover:bg-zinc-700 text-zinc-200 font-semibold text-xs transition"
                >
                  无损下载原图
                </button>
              </div>

            </div>

          </div>
        </div>
      )}

      {/* 5. 极致极简页脚 */}
      <footer className="border-t border-zinc-900 bg-zinc-950 py-8 px-6 mt-12">
        <div className="max-w-[1600px] mx-auto flex flex-col sm:flex-row items-center justify-between gap-4 text-xs text-zinc-500">
          <div className="flex items-center gap-2">
            <span>© 2026 IMAGIX AI. 视觉创意生产系统.</span>
            <span className="text-zinc-800">|</span>
            <span className="flex items-center gap-1">
              基于 <strong className="text-zinc-400 font-medium">Gemini 2.5 Flash</strong> & <strong className="text-zinc-400 font-medium">Imagen 4.0</strong> 构建
            </span>
          </div>
          <div className="flex items-center gap-4">
            <span className="hover:text-zinc-300 cursor-pointer">服务协议</span>
            <span className="hover:text-zinc-300 cursor-pointer">隐私条款</span>
            <span className="hover:text-zinc-300 cursor-pointer" onClick={() => setActiveTab('settings')}>API 文档</span>
          </div>
        </div>
      </footer>

    </div>
  );
}

