# 图片对客售价统一至路由模型与Seedream任务恢复

日期2026-10-26；来源本会话用户四项明确要求。保留所有未提交变更，不自动commit/push或远程部署。

## 要求

1. 查明Qwen/Seedream路由未配1/2/4K仍可估价创建的实际费用来源，解释成本/销售与积分兑换。禁止缺销售价时fallback到供应商成本或0价。
2. 所有对客图片售价统一由路由价格配置维护；供应商账号/精确型号仅维护成本及原生默认能力。支持按张输出1/2/4K适用档位、输入图片单价与首图免费，以及按像素token计费（明确单位/公式），不能为型号不支持的4K开放配置或生成。
3. 拆层费用成本与销售分离：成本接入型号、销售路由，底图和图层费率完整且明确确认后入口开放；配置欠缺时清楚列缺项，不用笼统提示或unknown=0。
4. 排查用户当前Seedream任务不结束；先查队列、lease、提交标记和Worker，无盲重试或创建重复付费任务。任务已明确由用户提交，恢复其原任务处理在本轮授权范围；启动Worker前核对所有claimable queue，避免带无关历史付费任务。使用已恢复持久runtime，不能改roles/concurrency绕绑定。

## 设计约束

- 调研后在现有route pricing实体与版本化extra间选择最小兼容扩展，复用公开API/类型/路由候选能力，不混用legacy base_points与native sale_pricing双源。
- 新任务报价/创建/实际结算统一route sale；freeze售价/成本/FX/实际参数到任务快照。已有任务使用原快照完成不被新售价重算；旧账号sale配置若迁移，需明确幂等、冲突保守、无静默改价，不永久runtime fallback。迁移前备份。
- 像素token模式明确token换算（不假称厂商usage等于pixels）、尺寸/输入尺寸必要元数据、预估与实际结算同公式，无法可靠计算时拒绝报价。按张与token不得混算；费率有限非负、必填未配不可放行。
- UI路由区统一售价表单，按当前候选真实能力交集展示支持档位与像素约束。输入免费张数配置支持首图免费；拆层底图/层与确认单独分区。账号移除销售编辑入口并给跳转说明，保未知extra。
- API更新使用runtime-local/e2e/config/runtime.env及配对state，预先确认文件存在/验证通过再停止旧API。Worker仅在队列安全检查后处理授权原任务，保持绑定。

## 实现契约

路由新增 `image_pricing` JSON字段（独立字段，不引入通用extra），复用route CRUD，缺失为未配置。结构为 `{schema_version:1,rules:Rule[]}`，每种operation最多一条规则。Rule包含 `operation:generate|edit|layer_decomposition,mode:per_image|pixel_token,currency:CNY|points,version:string,input:{free_images:非负整数,per_image?:十进制字符串},output_tiers?:[{name,max_pixels?:正整数,base_resolution?:string,price:十进制字符串}],pixel_token?:{definition:platform_pixels_v1,pixels_per_token:正整数,input_per_million:十进制字符串,output_per_million:十进制字符串},layers?:{confirmed:boolean,base_price?:十进制字符串,tiers:同tiers}`。max_pixels与base_resolution互斥：原生按实际像素首个匹配；legacy按原分辨率档位精确匹配，不能以新阈值改变旧价格语义。缺省价格和明确零价必须区分，不使用task_overrides。

像素token明示平台计量，不使用厂商usage：逐图tokens=ceil(width*height/pixels_per_token)，每任务前free_input_images张不收输入；合计tokens/1e6乘对应费率，不能合并像素后再ceil。reference dimensions来自服务端资产元数据；缺尺寸拒绝token估价。普通per_image允许不按输入尺寸。拆层单独底图与layer按张tier，不支持pixel_token拆层（UI明确且不放行），避免混算。

旧legacy route price仍属于路由销售来源，兼容旧API并在统一表单展示；有新image_pricing优先，新native请求必须route image_pricing，否则拒绝，不fallback账号sale。原生旧sale统一迁移工具：仅所有有效候选已配置且sale值相同、route未有冲突价格时可幂等迁移；不同候选售价拒迁并报告。旧任务pricing snapshot无新route字段走旧结算，新task快照冻结统一route config及候选成本。不得静默改变币种或数额。用户端支持档位来自有效模型实际能力与价格匹配组合。

## 验证

成本/售价独立、缺路由价格拒绝、旧任务快照兼容、迁移重复/冲突、1/2K和不支持4K、输入首免、token边界与实际结算、拆层确认各缺项；Go/TS聚焦test、完整verify、独立review、隔离smoke与浏览器后台表单/用户报价。不得凭模拟调用宣称真实上游成功；真实任务状态与扣款如实记录，不泄露secret或媒体签名URL。
