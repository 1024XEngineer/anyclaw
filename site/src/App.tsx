import { Link, NavLink, Route, Routes, useNavigate, useParams } from "react-router-dom";
import {
  AlertCircle,
  ArrowRight,
  CheckCircle2,
  Clipboard,
  ClipboardCheck,
  Download,
  ExternalLink,
  FileArchive,
  Github,
  Hash,
  HelpCircle,
  Menu,
  PackageSearch,
  RefreshCw,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  X
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import {
  docLinks,
  downloadCommands,
  downloadOptions,
  faqItems,
  featureBlocks,
  installChecklist,
  kindIcons,
  kindLabels,
  marketplaceInstallSteps,
  publishCommands,
  publishLifecycle,
  quickstartSteps,
  releaseAssets,
  supportLinks,
  type MarketKind,
  type MarketItem,
  type MarketVersion
} from "./data";
import { getRegistryArtifact, listRegistryArtifactVersions, listRegistryArtifacts } from "./registryClient";
import { AdminPage } from "./AdminPage";

const navItems = [
  { to: "/marketplace", label: "市场" },
  { to: "/download", label: "下载" },
  { to: "/docs", label: "文档" },
  { to: "/publish", label: "发布" }
];

function App() {
  return (
    <div className="app-shell">
      <Header />
      <main>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/marketplace" element={<MarketplacePage />} />
          <Route path="/marketplace/:artifactId" element={<ArtifactDetailPage />} />
          <Route path="/download" element={<DownloadPage />} />
          <Route path="/docs" element={<DocsPage />} />
          <Route path="/publish" element={<PublishPage />} />
          <Route path="/admin" element={<AdminPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </main>
      <Footer />
    </div>
  );
}

function Header() {
  const [open, setOpen] = useState(false);

  return (
    <header className="site-header">
      <Link to="/" className="brand" onClick={() => setOpen(false)}>
        <span className="brand-mark">A</span>
        <span>AnyClaw</span>
      </Link>
      <button
        type="button"
        className="icon-button nav-toggle"
        onClick={() => setOpen((value) => !value)}
        aria-label={open ? "关闭导航" : "打开导航"}
      >
        {open ? <X size={20} /> : <Menu size={20} />}
      </button>
      <nav className={open ? "site-nav open" : "site-nav"} aria-label="主导航">
        {navItems.map((item) => (
          <NavLink key={item.to} to={item.to} onClick={() => setOpen(false)}>
            {item.label}
          </NavLink>
        ))}
        <a href="https://github.com/1024XEngineer/anyclaw" target="_blank" rel="noreferrer">
          <Github size={17} />
          GitHub
        </a>
      </nav>
    </header>
  );
}

function HomePage() {
  return (
    <>
      <section className="hero-section">
        <div className="hero-content">
          <p className="eyebrow">Local-first Agent Workspace</p>
          <h1>AnyClaw</h1>
          <p className="hero-subtitle">本地优先的 AI Agent 工作台</p>
          <p className="hero-copy">
            把模型、工具、工作区和能力市场接在一起。让 AI 不只回答问题，也能在你的本地环境里调用工具、操作文件、连接浏览器，并通过 Agent / Skill / CLI 扩展完成真实任务。
          </p>
          <div className="hero-actions">
            <Link className="primary-button" to="/download">
              <Download size={18} />
              下载 AnyClaw
            </Link>
            <Link className="secondary-button" to="/marketplace">
              <PackageSearch size={18} />
              浏览云端市场
            </Link>
          </div>
          <div className="status-row" aria-label="能力状态">
            <span>Local-first</span>
            <span>Agent / Skill / CLI</span>
            <span>Policy controlled install</span>
            <span>Registry ready</span>
          </div>
        </div>
        <ProductPreview />
      </section>

      <section className="content-band">
        <div className="section-heading">
          <p className="eyebrow">Workflow</p>
          <h2>先找能力，再安装，再绑定。</h2>
          <p>Registry 负责分发，本地 AnyClaw 负责最终决策。安装、绑定、升级、卸载都有明确状态。</p>
        </div>
        <div className="feature-grid">
          {featureBlocks.map((item) => (
            <article className="feature-card" key={item.title}>
              <item.icon size={24} />
              <h3>{item.title}</h3>
              <p>{item.description}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="split-section">
        <div>
          <p className="eyebrow">Quickstart</p>
          <h2>从本地开始，逐步接入市场。</h2>
          <p>
            第一版官网不伪装已有安装包。当前推荐路径是从源码构建 CLI，完成 onboard 和 doctor，再进入交互模式。
          </p>
        </div>
        <ol className="step-list">
          {quickstartSteps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
      </section>
    </>
  );
}

function ProductPreview() {
  return (
    <div className="product-preview" aria-label="AnyClaw 产品界面预览">
      <div className="preview-toolbar">
        <span>Gateway</span>
        <span className="preview-pill online">online</span>
      </div>
      <div className="preview-grid">
        <div className="preview-sidebar">
          <span className="active">市场</span>
          <span>Workspace</span>
          <span>Events</span>
          <span>Policy</span>
        </div>
        <div className="preview-main">
          <div className="preview-line wide" />
          <div className="preview-market-row">
            <span>发布说明</span>
            <span className="preview-pill">技能</span>
          </div>
          <div className="preview-market-row">
            <span>发布管理</span>
            <span className="preview-pill">代理</span>
          </div>
          <div className="preview-market-row">
            <span>工作区诊断</span>
            <span className="preview-pill">命令行</span>
          </div>
        </div>
      </div>
    </div>
  );
}

function MarketplacePage() {
  const navigate = useNavigate();
  const [kind, setKind] = useState<MarketKind | "all">("all");
  const [query, setQuery] = useState("");
  const [risk, setRisk] = useState("");
  const [trust, setTrust] = useState("");
  const [tag, setTag] = useState("");
  const [permission, setPermission] = useState("");
  const [publisher, setPublisher] = useState("");
  const [os, setOS] = useState("");
  const [arch, setArch] = useState("");
  const [sort, setSort] = useState("score");
  const [items, setItems] = useState<MarketItem[]>([]);
  const [status, setStatus] = useState<"loading" | "live" | "fallback">("loading");
  const [error, setError] = useState<string | null>(null);
  const [endpoint, setEndpoint] = useState("/v1/artifacts");
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    const fallbackTimer = window.setTimeout(() => {
      controller.abort();
      setItems([]);
      setEndpoint("/v1/artifacts");
      setStatus("fallback");
      setError("Registry 请求超时");
    }, 6000);

    setStatus("loading");
    setError(null);

    listRegistryArtifacts({ arch, kind, os, permission, publisher, q: query, risk, sort, tag, trust }, controller.signal)
      .then((result) => {
        window.clearTimeout(fallbackTimer);
        setItems(result.items);
        setEndpoint(result.endpoint);
        setStatus("live");
      })
      .catch((err: unknown) => {
        window.clearTimeout(fallbackTimer);
        if (controller.signal.aborted) {
          return;
        }
        setItems([]);
        setEndpoint("/v1/artifacts");
        setStatus("fallback");
        setError(err instanceof Error ? err.message : "Registry 暂时不可达");
      });

    return () => {
      window.clearTimeout(fallbackTimer);
      controller.abort();
    };
  }, [arch, kind, os, permission, publisher, query, refreshKey, risk, sort, tag, trust]);

  const filteredItems = useMemo(() => items, [items]);
  const counts = useMemo(
    () => ({
      agent: items.filter((item) => item.kind === "agent").length,
      all: items.length,
      cli: items.filter((item) => item.kind === "cli").length,
      skill: items.filter((item) => item.kind === "skill").length
    }),
    [items]
  );

  return (
    <PageShell
      eyebrow="云端市场"
      title="代理 / 技能 / 命令行的云端市场"
      description="公开官网读取真实 Registry，展示可安装能力、权限、安全元数据和安装指引。这里不保存 token，也不直接操作本地客户端。"
    >
      <RegistryStatus
        status={status}
        endpoint={endpoint}
        error={error}
        hasItems={items.length > 0}
        onRefresh={() => setRefreshKey((key) => key + 1)}
      />
      <div className="market-summary-strip">
        <div>
          <strong>{counts.all}</strong>
          <span>全部条目</span>
        </div>
        <div>
          <strong>{counts.agent}</strong>
          <span>代理</span>
        </div>
        <div>
          <strong>{counts.skill}</strong>
          <span>技能</span>
        </div>
        <div>
          <strong>{counts.cli}</strong>
          <span>命令行</span>
        </div>
      </div>
      <div className="market-controls">
        <div className="segmented" role="tablist" aria-label="资源类型">
          {(["all", "agent", "skill", "cli"] as const).map((value) => (
            <button
              key={value}
              type="button"
              className={kind === value ? "active" : ""}
              onClick={() => setKind(value)}
            >
              {value === "all" ? "全部" : kindLabels[value]}
            </button>
          ))}
        </div>
        <label className="search-box">
          <Search size={18} />
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索能力" />
        </label>
      </div>
      <div className="market-filter-grid">
        <label>
          <span>风险</span>
          <select value={risk} onChange={(event) => setRisk(event.target.value)}>
            <option value="">全部</option>
            <option value="low">low</option>
            <option value="medium">medium</option>
            <option value="high">high</option>
            <option value="unknown">unknown</option>
          </select>
        </label>
        <label>
          <span>可信度</span>
          <select value={trust} onChange={(event) => setTrust(event.target.value)}>
            <option value="">全部</option>
            <option value="verified">verified</option>
            <option value="community">community</option>
            <option value="unverified">unverified</option>
            <option value="unknown">unknown</option>
          </select>
        </label>
        <label>
          <span>排序</span>
          <select value={sort} onChange={(event) => setSort(event.target.value)}>
            <option value="score">综合评分</option>
            <option value="updated">最近更新</option>
            <option value="name">名称 A-Z</option>
          </select>
        </label>
        <label>
          <span>标签</span>
          <input value={tag} onChange={(event) => setTag(event.target.value)} placeholder="例如 marketplace" />
        </label>
        <label>
          <span>权限</span>
          <input value={permission} onChange={(event) => setPermission(event.target.value)} placeholder="例如 fs.read" />
        </label>
        <label>
          <span>发布者</span>
          <input value={publisher} onChange={(event) => setPublisher(event.target.value)} placeholder="发布者名称" />
        </label>
        <label>
          <span>系统</span>
          <input value={os} onChange={(event) => setOS(event.target.value)} placeholder="windows / linux / darwin" />
        </label>
        <label>
          <span>架构</span>
          <input value={arch} onChange={(event) => setArch(event.target.value)} placeholder="amd64 / arm64" />
        </label>
      </div>
      <div className="install-guide-panel">
        <div>
          <Sparkles size={22} />
          <strong>安装在客户端完成</strong>
          <span>官网负责看清楚能力和风险；真正安装、绑定、升级、卸载都在 AnyClaw 客户端里执行。</span>
        </div>
        <ol>
          {marketplaceInstallSteps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
      </div>
      {status === "loading" && items.length === 0 ? (
        <div className="market-grid" aria-label="正在加载市场数据">
          {Array.from({ length: 3 }).map((_, index) => (
            <article className="market-card skeleton-card" key={index}>
              <div className="skeleton-line title" />
              <div className="skeleton-line" />
              <div className="skeleton-line short" />
            </article>
          ))}
        </div>
      ) : filteredItems.length > 0 ? (
        <div className="market-grid">
          {filteredItems.map((item) => (
            <MarketCard key={item.id} item={item} onSelect={() => navigate(`/marketplace/${encodeURIComponent(item.id)}`)} />
          ))}
        </div>
      ) : (
        <div className="empty-state">
          <PackageSearch size={24} />
          <p>没有找到匹配的能力。</p>
        </div>
      )}
    </PageShell>
  );
}

function RegistryStatus({
  status,
  endpoint,
  error,
  hasItems,
  onRefresh
}: {
  status: "loading" | "live" | "fallback";
  endpoint: string;
  error: string | null;
  hasItems: boolean;
  onRefresh: () => void;
}) {
  const live = status === "live";
  const message =
    status === "loading"
      ? hasItems
        ? "正在刷新 Registry，先展示当前云端数据"
        : "正在连接 Registry"
      : live
        ? `Registry 已连接：${endpoint}`
        : `Registry 不可达：${error ?? "未知错误"}`;

  return (
    <div className={`registry-status ${status}`}>
      <div>
        {live ? <CheckCircle2 size={20} /> : status === "loading" ? <RefreshCw size={20} /> : <AlertCircle size={20} />}
        <span>{message}</span>
      </div>
      <button type="button" className="secondary-button compact" onClick={onRefresh}>
        <RefreshCw size={16} />
        刷新
      </button>
    </div>
  );
}

function MarketCard({ item, onSelect }: { item: MarketItem; onSelect: () => void }) {
  const Icon = kindIcons[item.kind];
  return (
    <article className="market-card">
      <div className="card-title-row">
        <span className="kind-icon">
          <Icon size={18} />
        </span>
        <div>
          <h3>{item.name}</h3>
          <p>{item.id}</p>
        </div>
      </div>
      <p className="card-summary">{item.summary}</p>
      <div className="meta-row">
        <span>{kindLabels[item.kind]}</span>
        <span>v{item.version}</span>
        <span>{item.source}</span>
        {item.publisher ? <span>{item.publisher}</span> : null}
      </div>
      <div className="badge-row">
        <span className={`badge ${item.risk}`}>风险：{item.riskLabel ?? item.risk}</span>
        <span className={`badge ${item.trust}`}>可信度：{item.trustLabel ?? item.trust}</span>
      </div>
      <div className="permission-row">
        {item.permissions.length > 0 ? (
          item.permissions.slice(0, 3).map((permission) => <span key={permission}>{permission}</span>)
        ) : (
          <span>未声明权限</span>
        )}
      </div>
      {item.hitSignals && item.hitSignals.length > 0 ? (
        <div className="signal-row">
          {item.hitSignals.slice(0, 3).map((signal) => (
            <span key={signal}>{signal}</span>
          ))}
        </div>
      ) : null}
      <button type="button" className="detail-button" onClick={onSelect}>
        查看详情与安装指引
      </button>
    </article>
  );
}

function ArtifactDetailPage() {
  const { artifactId = "" } = useParams();
  const decodedID = decodeURIComponent(artifactId);
  const [item, setItem] = useState<MarketItem | null>(null);
  const [versions, setVersions] = useState<MarketVersion[]>([]);
  const [status, setStatus] = useState<"loading" | "live" | "error">("loading");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!decodedID) {
      setStatus("error");
      setError("缺少 artifact id");
      return;
    }
    const controller = new AbortController();
    setStatus("loading");
    setError(null);
    Promise.all([getRegistryArtifact(decodedID, controller.signal), listRegistryArtifactVersions(decodedID, controller.signal)])
      .then(([detail, versionItems]) => {
        if (!detail.item) {
          setStatus("error");
          setError("没有找到这个 artifact");
          return;
        }
        setItem(detail.item);
        setVersions(versionItems);
        setStatus("live");
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return;
        setItem(null);
        setVersions([]);
        setStatus("error");
        setError(err instanceof Error ? err.message : "详情暂时不可用");
      });
    return () => controller.abort();
  }, [decodedID]);

  return (
    <PageShell eyebrow="Artifact" title={item?.name ?? decodedID} description="公开详情页展示 Registry 元数据、版本、权限和客户端安装方式。">
      <div className="detail-route-actions">
        <Link className="secondary-button" to="/marketplace">
          <ArrowRight size={18} />
          返回市场
        </Link>
        <Link className="primary-button" to="/download">
          <Download size={18} />
          获取 AnyClaw 客户端
        </Link>
      </div>
      {status === "loading" ? (
        <div className="detail-panel">
          <div className="skeleton-line title" />
          <div className="skeleton-line" />
          <div className="skeleton-line short" />
        </div>
      ) : item ? (
        <MarketDetailPanel item={item} versions={versions} versionsError={null} />
      ) : (
        <div className="empty-state">
          <AlertCircle size={24} />
          <p>{error ?? "详情暂时不可用"}</p>
        </div>
      )}
    </PageShell>
  );
}

function MarketDetailPanel({
  item,
  versions,
  versionsError
}: {
  item: MarketItem;
  versions: MarketVersion[];
  versionsError: string | null;
}) {
  const Icon = kindIcons[item.kind];
  const os = item.compatibility?.os?.join(" / ") || "未声明";
  const arch = item.compatibility?.arch?.join(" / ") || "未声明";
  const size = typeof item.sizeBytes === "number" && item.sizeBytes > 0 ? formatBytes(item.sizeBytes) : "未声明";

  return (
    <div className="detail-panel" aria-label={`${item.name} 详情`}>
      <div className="detail-panel-header">
        <div className="card-title-row">
          <span className="kind-icon">
            <Icon size={18} />
          </span>
          <div>
            <h3>{item.name}</h3>
            <p>{item.id}</p>
          </div>
        </div>
      </div>
      <p>{item.summary}</p>
      <dl className="detail-list">
        <div>
          <dt>类型</dt>
          <dd>{kindLabels[item.kind]}</dd>
        </div>
        <div>
          <dt>版本</dt>
          <dd>v{item.version}</dd>
        </div>
        <div>
          <dt>发布者</dt>
          <dd>{item.publisher || "未知"}</dd>
        </div>
        <div>
          <dt>风险 / 可信度</dt>
          <dd>
            {item.riskLabel ?? item.risk} / {item.trustLabel ?? item.trust}
          </dd>
        </div>
        <div>
          <dt>AnyClaw min</dt>
          <dd>{item.compatibility?.anyclaw_min || "未声明"}</dd>
        </div>
        <div>
          <dt>OS</dt>
          <dd>{os}</dd>
        </div>
        <div>
          <dt>Arch</dt>
          <dd>{arch}</dd>
        </div>
        <div>
          <dt>包大小</dt>
          <dd>{size}</dd>
        </div>
      </dl>
      <div className="detail-section">
        <strong>权限</strong>
        <div className="permission-row">
          {item.permissions.length > 0 ? (
            item.permissions.map((permission) => <span key={permission}>{permission}</span>)
          ) : (
            <span>未声明权限</span>
          )}
        </div>
      </div>
      {item.tags && item.tags.length > 0 ? (
        <div className="detail-section">
          <strong>标签</strong>
          <div className="signal-row">
            {item.tags.map((tag) => (
              <span key={tag}>{tag}</span>
            ))}
          </div>
        </div>
      ) : null}
      <div className="detail-section">
        <strong>客户端安装指引</strong>
        <div className="install-command-box">
          <code>打开 AnyClaw 客户端 → 能力市场 → 云端 → 搜索 {item.id} → 查看权限 → 安装 → 绑定</code>
        </div>
      </div>
      <div className="detail-section">
        <strong>版本</strong>
        {versionsError ? <p className="detail-note">{versionsError}</p> : null}
        {versions.length > 0 ? (
          <div className="version-list">
            {versions.map((version) => (
              <div key={version.version}>
                <span>v{version.version}</span>
                <small>
                  {version.releasedAt || "未声明"} · {version.sizeBytes ? formatBytes(version.sizeBytes) : "未声明"}
                </small>
                {version.changelog ? <p>{version.changelog}</p> : null}
                {version.permissionsDiff && version.permissionsDiff.length > 0 ? (
                  <div className="permission-row">
                    {version.permissionsDiff.map((permission) => (
                      <span key={permission}>{permission}</span>
                    ))}
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        ) : versionsError ? null : (
          <p className="detail-note">暂无版本元数据。</p>
        )}
      </div>
    </div>
  );
}

function formatBytes(value: number) {
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function DownloadPage() {
  return (
    <PageShell
      eyebrow="Download"
      title="下载和安装 AnyClaw"
      description="当前公开仓库还没有正式 GitHub Release，所以官网优先给出真实可用的源码构建路径；tag 发布后会自动生成 zip / tar.gz 和 sha256。"
    >
      <div className="download-summary">
        <div>
          <strong>当前可用</strong>
          <span>源码构建 + 复制命令</span>
        </div>
        <div>
          <strong>Release 准备中</strong>
          <span>v* tag 自动打包</span>
        </div>
        <div>
          <strong>校验方式</strong>
          <span>每个产物旁放 .sha256</span>
        </div>
      </div>
      <div className="download-option-grid">
        {downloadOptions.map((option) => (
          <article className="download-option" key={option.platform}>
            <div>
              <h3>{option.platform}</h3>
              <span className={`badge ${option.status}`}>{option.status}</span>
            </div>
            <p>{option.description}</p>
            {option.releaseAsset ? (
              <small>
                Release asset: <code>{option.releaseAsset}</code>
              </small>
            ) : null}
          </article>
        ))}
      </div>
      <div className="download-grid">
        <CommandBlock title="Windows" language="powershell" command={downloadCommands.windows} />
        <CommandBlock title="macOS / Linux" language="bash" command={downloadCommands.unix} />
        <CommandBlock title="Docker Compose Gateway" language="bash" command={downloadCommands.docker} />
      </div>
      <div className="split-section download-details">
        <div>
          <p className="eyebrow">Release Assets</p>
          <h2>正式包不会手工乱传。</h2>
          <p>
            新增的 release workflow 会在推送 `v*` tag 时构建平台包，并上传 `.sha256`。在第一个正式 tag 发布前，下载页不放死链。
          </p>
          <a
            className="secondary-button"
            href="https://github.com/1024XEngineer/anyclaw/releases"
            target="_blank"
            rel="noreferrer"
          >
            <ExternalLink size={18} />
            查看 GitHub Releases
          </a>
        </div>
        <div className="release-panel">
          <div className="release-panel-header">
            <FileArchive size={22} />
            <strong>计划产物</strong>
          </div>
          <ul>
            {releaseAssets.map((asset) => (
              <li key={asset}>
                <code>{asset}</code>
              </li>
            ))}
          </ul>
          <div className="checksum-note">
            <Hash size={18} />
            <span>每个文件都会生成同名 `.sha256`，用于下载后校验。</span>
          </div>
        </div>
      </div>
      <div className="content-band compact-band">
        <div className="section-heading">
          <p className="eyebrow">Install Checklist</p>
          <h2>安装后先跑 doctor。</h2>
        </div>
        <ol className="step-list">
          {installChecklist.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
      </div>
      <div className="content-band compact-band">
        <div className="section-heading">
          <p className="eyebrow">Verification</p>
          <h2>下载后校验 SHA256。</h2>
          <p>正式 release 产物旁会放同名 `.sha256`。源码构建阶段也建议对下载的 release 包做 hash 校验后再运行。</p>
        </div>
        <div className="download-grid">
          <CommandBlock title="Windows checksum" language="powershell" command={`Get-FileHash .\\anyclaw_VERSION_windows_amd64.zip -Algorithm SHA256\nGet-Content .\\anyclaw_VERSION_windows_amd64.zip.sha256`} />
          <CommandBlock title="macOS / Linux checksum" language="bash" command={`shasum -a 256 anyclaw_VERSION_linux_amd64.tar.gz\ncat anyclaw_VERSION_linux_amd64.tar.gz.sha256`} />
        </div>
      </div>
    </PageShell>
  );
}

function CommandBlock({ title, language, command }: { title: string; language: string; command: string }) {
  const [copied, setCopied] = useState(false);

  async function copyCommand() {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  }

  return (
    <article className="command-card">
      <div className="command-header">
        <div>
          <h3>{title}</h3>
          <span>{language}</span>
        </div>
        <button type="button" className="icon-button" onClick={copyCommand} aria-label={`复制 ${title} 命令`}>
          {copied ? <ClipboardCheck size={18} /> : <Clipboard size={18} />}
        </button>
      </div>
      <pre>
        <code>{command}</code>
      </pre>
    </article>
  );
}

function DocsPage() {
  return (
    <PageShell
      eyebrow="Docs"
      title="文档、FAQ 和更新日志"
      description="公开官网保留清晰入口，核心说明仍链接到真实仓库，避免官网和仓库文档后续漂移。"
    >
      <div className="doc-grid">
        {docLinks.map((item) => (
          <a className="doc-link" key={item.title} href={item.href} target="_blank" rel="noreferrer">
            <item.icon size={22} />
            <span>
              <strong>{item.title}</strong>
              <small>{item.description}</small>
            </span>
            <ExternalLink size={17} />
          </a>
        ))}
      </div>
      <section className="faq-section" id="faq">
        <div className="section-heading">
          <p className="eyebrow">FAQ</p>
          <h2>上线前最容易问到的几个问题</h2>
        </div>
        <div className="faq-grid">
          {faqItems.map((item) => (
            <article className="faq-card" key={item.question}>
              <HelpCircle size={20} />
              <h3>{item.question}</h3>
              <p>{item.answer}</p>
            </article>
          ))}
        </div>
      </section>
      <section className="content-band compact-band">
        <div className="section-heading">
          <p className="eyebrow">Changelog</p>
          <h2>更新日志入口</h2>
          <p>版本更新以仓库 release 和 artifact version changelog 为准。市场详情页会展示 registry 返回的版本 changelog。</p>
        </div>
        <div className="doc-grid">
          {supportLinks.map((item) =>
            item.href.startsWith("/") ? (
              <Link className="doc-link" key={item.title} to={item.href}>
                <item.icon size={22} />
                <span>
                  <strong>{item.title}</strong>
                  <small>{item.description}</small>
                </span>
                <ArrowRight size={17} />
              </Link>
            ) : (
              <a className="doc-link" key={item.title} href={item.href} target="_blank" rel="noreferrer">
                <item.icon size={22} />
                <span>
                  <strong>{item.title}</strong>
                  <small>{item.description}</small>
                </span>
                <ExternalLink size={17} />
              </a>
            )
          )}
        </div>
      </section>
    </PageShell>
  );
}

function PublishPage() {
  return (
    <PageShell
      eyebrow="Publish"
      title="发布流程围绕 Token、审计和 quarantine"
      description="管理员保管 admin token，发布者使用 publisher token。Registry 负责接收包、记录审计、提供下载和隔离能力。"
    >
      <div className="feature-grid">
        {publishLifecycle.map((item) => (
          <article className="feature-card" key={item.title}>
            <item.icon size={24} />
            <h3>{item.title}</h3>
            <p>{item.description}</p>
          </article>
        ))}
      </div>
      <div className="download-grid publish-command-grid">
        <CommandBlock title="创建 publisher token" language="powershell" command={publishCommands.createToken} />
        <CommandBlock title="发布示例 skill" language="powershell" command={publishCommands.publishSkill} />
      </div>
      <div className="callout">
        <ShieldCheck size={22} />
        <p>
          admin token 不写进官网，也不交给发布者。生产部署时必须通过服务器环境变量注入，并配合 registry audit 检查发布和管理操作。
        </p>
      </div>
      <div className="doc-grid publish-doc-grid">
        <a className="doc-link" href="https://github.com/1024XEngineer/anyclaw/blob/main/docs/PUBLISHING.md" target="_blank" rel="noreferrer">
          <FileArchive size={22} />
          <span>
            <strong>Publishing Guide</strong>
            <small>查看 manifest 模板、token 流程、publish smoke checklist 和当前包生成限制。</small>
          </span>
          <ExternalLink size={17} />
        </a>
      </div>
    </PageShell>
  );
}

function NotFoundPage() {
  return (
    <PageShell eyebrow="404" title="这个页面还没有接入" description="可以回到首页，或进入云端市场和下载页继续浏览。">
      <div className="hero-actions">
        <Link className="primary-button" to="/">
          <ArrowRight size={18} />
          回到首页
        </Link>
        <Link className="secondary-button" to="/marketplace">
          <PackageSearch size={18} />
          浏览市场
        </Link>
      </div>
    </PageShell>
  );
}

function PageShell({
  eyebrow,
  title,
  description,
  children
}: {
  eyebrow: string;
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <section className="page-section">
      <div className="section-heading page-heading">
        <p className="eyebrow">{eyebrow}</p>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {children}
    </section>
  );
}

function Footer() {
  return (
    <footer className="site-footer">
      <div>
        <strong>AnyClaw</strong>
        <span>本地优先的 AI Agent 工作台</span>
      </div>
      <div className="footer-links">
        <Link to="/docs">文档</Link>
        <Link to="/publish">发布</Link>
        <a href="https://github.com/1024XEngineer/anyclaw" target="_blank" rel="noreferrer">
          GitHub
        </a>
      </div>
      <div className="footer-note">
        <CheckCircle2 size={16} />
        <span>市场通过 /v1/artifacts 读取实时 Registry 数据。</span>
      </div>
    </footer>
  );
}

export default App;
