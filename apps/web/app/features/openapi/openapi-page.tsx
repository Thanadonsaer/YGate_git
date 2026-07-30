"use client";

import { Braces, Check, Copy, Download, FileCode2, RefreshCw, Search, ShieldCheck } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { parse } from "yaml";
import { iconButtonClass, inputClass, primaryButtonClass } from "../../components/ui";
import { api, csrfToken } from "../../lib/api";

type HttpMethod = "get" | "post" | "put" | "patch" | "delete";
type Operation = {
  method: HttpMethod;
  path: string;
  operationId: string;
  summary: string;
  tags: string[];
  secured: boolean;
  description: string;
  parameters: Parameter[];
  requestBody?: unknown;
  responses?: unknown;
  securityNames: string[];
};
type Parameter = { $ref?: string; name?: string; in?: "path" | "query" | "header"; required?: boolean; description?: string; schema?: unknown };
type OpenAPIOperation = {
  operationId?: string;
  summary?: string;
  description?: string;
  tags?: string[];
  security?: Record<string, unknown>[];
  parameters?: Parameter[];
  requestBody?: unknown;
  responses?: unknown;
};
type OpenAPIDocument = {
  openapi?: string;
  info?: { title?: string; version?: string; description?: string };
  paths?: Record<string, { parameters?: Parameter[] } & Partial<Record<HttpMethod, OpenAPIOperation>>>;
  components?: { parameters?: Record<string, Parameter>; securitySchemes?: Record<string, unknown> };
};

const methods: HttpMethod[] = ["get", "post", "put", "patch", "delete"];
const methodStyles: Record<HttpMethod, string> = {
  get: "bg-sky-50 text-sky-700",
  post: "bg-emerald-50 text-emerald-700",
  put: "bg-amber-50 text-amber-800",
  patch: "bg-violet-50 text-violet-700",
  delete: "bg-rose-50 text-rose-700",
};

export function OpenAPIPage() {
  const [source, setSource] = useState("");
  const [query, setQuery] = useState("");
  const [method, setMethod] = useState<HttpMethod | "all">("all");
  const [tab, setTab] = useState<"endpoints" | "source">("endpoints");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");

  const loadDocument = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/admin/openapi");
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดู OpenAPI");
      if (!response.ok) throw new Error("ไม่สามารถโหลด OpenAPI contract ได้");
      setSource(await response.text());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadDocument(); }, [loadDocument]);

  const parsed = useMemo(() => {
    if (!source) return { document: null, operations: [] as Operation[], parseError: "" };
    try {
      const document = parse(source) as OpenAPIDocument;
      const operations = Object.entries(document.paths ?? {}).flatMap(([path, pathItem]) =>
        methods.flatMap((currentMethod) => {
          const operation = pathItem[currentMethod];
          if (!operation) return [];
          const securityNames = (operation.security ?? []).flatMap((item) => Object.keys(item));
          const parameters = [...(pathItem.parameters ?? []), ...(operation.parameters ?? [])].map((parameter) =>
            parameter.$ref?.startsWith("#/components/parameters/")
              ? document.components?.parameters?.[parameter.$ref.slice(24)] ?? parameter
              : parameter
          );
          return [{
            method: currentMethod,
            path,
            operationId: operation.operationId ?? "-",
            summary: operation.summary ?? "ไม่มีคำอธิบาย",
            tags: operation.tags ?? [],
            secured: Boolean(operation.security?.length),
            description: operation.description ?? "",
            parameters,
            requestBody: operation.requestBody,
            responses: operation.responses,
            securityNames,
          }];
        })
      );
      return { document, operations, parseError: "" };
    } catch (cause) {
      return { document: null, operations: [] as Operation[], parseError: cause instanceof Error ? cause.message : "YAML ไม่ถูกต้อง" };
    }
  }, [source]);

  const normalizedQuery = query.trim().toLocaleLowerCase();
  const filtered = parsed.operations.filter((operation) =>
    (method === "all" || operation.method === method) &&
    (!normalizedQuery || [operation.path, operation.summary, operation.operationId, ...operation.tags]
      .some((value) => value.toLocaleLowerCase().includes(normalizedQuery)))
  );
  const securitySchemeCount = Object.keys(parsed.document?.components?.securitySchemes ?? {}).length;

  function download() {
    const link = window.document.createElement("a");
    link.href = URL.createObjectURL(new Blob([source], { type: "application/yaml" }));
    link.download = "platform-api.yaml";
    link.click();
    URL.revokeObjectURL(link.href);
  }

  async function copy(value: string) {
    await navigator.clipboard.writeText(value);
    setCopied(value);
    window.setTimeout(() => setCopied(""), 1500);
  }

  return (
    <div className="content space-y-4">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p className="text-xs font-extrabold uppercase text-slate-500">API contract registry</p>
          <h2 className="text-2xl font-extrabold text-slate-900">{parsed.document?.info?.title ?? "Platform API"}</h2>
          <p className="mt-1 text-sm text-slate-500">สำรวจ endpoint และดาวน์โหลด contract ที่ใช้งานอยู่</p>
        </div>
        <div className="flex items-center gap-2">
          <button className={iconButtonClass} type="button" onClick={() => void loadDocument()} title="รีเฟรช Contract" aria-label="รีเฟรช Contract"><RefreshCw size={18} /></button>
          <button className={primaryButtonClass} type="button" onClick={download} disabled={!source}><Download size={17} /> ดาวน์โหลด YAML</button>
        </div>
      </div>

      {error && <p className="rounded-md bg-rose-50 px-3 py-2 text-sm font-bold text-rose-700">{error}</p>}
      {parsed.parseError && <p className="rounded-md bg-rose-50 px-3 py-2 text-sm font-bold text-rose-700">อ่าน YAML ไม่สำเร็จ: {parsed.parseError}</p>}

      <section className="grid gap-px overflow-hidden rounded-md border border-slate-200 bg-slate-200 sm:grid-cols-3" aria-label="OpenAPI summary">
        <div className="bg-white p-4"><span className="flex items-center gap-2 text-xs font-bold text-slate-500"><FileCode2 size={16} /> Specification</span><strong className="mt-2 block text-xl text-slate-900">OpenAPI {parsed.document?.openapi ?? "-"}</strong><small className="text-slate-500">Version {parsed.document?.info?.version ?? "-"}</small></div>
        <div className="bg-white p-4"><span className="flex items-center gap-2 text-xs font-bold text-slate-500"><Braces size={16} /> Operations</span><strong className="mt-2 block text-xl text-slate-900">{parsed.operations.length.toLocaleString()}</strong><small className="text-slate-500">{Object.keys(parsed.document?.paths ?? {}).length.toLocaleString()} unique paths</small></div>
        <div className="bg-white p-4"><span className="flex items-center gap-2 text-xs font-bold text-slate-500"><ShieldCheck size={16} /> Security schemes</span><strong className="mt-2 block text-xl text-slate-900">{securitySchemeCount.toLocaleString()}</strong><small className="text-slate-500">ประกาศใน contract</small></div>
      </section>

      <div className="border-b border-slate-200">
        <div className="flex gap-5" role="tablist" aria-label="OpenAPI views">
          <button className={`border-b-2 px-1 py-3 text-sm font-bold ${tab === "endpoints" ? "border-cyan-700 text-cyan-800" : "border-transparent text-slate-500"}`} type="button" role="tab" aria-selected={tab === "endpoints"} onClick={() => setTab("endpoints")}>Endpoints</button>
          <button className={`border-b-2 px-1 py-3 text-sm font-bold ${tab === "source" ? "border-cyan-700 text-cyan-800" : "border-transparent text-slate-500"}`} type="button" role="tab" aria-selected={tab === "source"} onClick={() => setTab("source")}>YAML Source</button>
        </div>
      </div>

      {tab === "endpoints" ? (
        <>
          <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <label className="relative min-w-0 xl:w-96">
              <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" size={17} />
              <span className="sr-only">ค้นหา endpoint</span>
              <input className={`${inputClass} pl-9`} type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="ค้นหา path, operation ID, summary..." />
            </label>
            <div className="flex max-w-full overflow-x-auto rounded-md border border-slate-200 bg-white p-1" role="group" aria-label="กรอง HTTP method">
              {(["all", ...methods] as const).map((item) => (
                <button key={item} className={`h-8 shrink-0 rounded px-3 text-xs font-extrabold uppercase ${method === item ? "bg-slate-900 text-white" : "text-slate-600 hover:bg-slate-50"}`} type="button" onClick={() => setMethod(item)}>{item}</button>
              ))}
            </div>
          </div>

          <section className="overflow-hidden rounded-md border border-slate-200 bg-white" aria-label="OpenAPI endpoints">
            {filtered.map((operation) => (
              <details key={`${operation.method}:${operation.path}`} className="group border-b border-slate-200 last:border-b-0">
                <summary className="grid cursor-pointer list-none grid-cols-[64px_minmax(0,1fr)_40px] items-center gap-3 px-4 py-3 hover:bg-slate-50 md:grid-cols-[64px_minmax(220px,1.1fr)_minmax(220px,1fr)_180px_40px]">
                  <span className={`rounded px-2 py-1 text-center text-xs font-black uppercase ${methodStyles[operation.method]}`}>{operation.method}</span>
                  <code className="min-w-0 truncate text-sm font-bold text-slate-900">{operation.path}</code>
                  <span className="hidden truncate text-sm text-slate-600 md:block">{operation.summary}</span>
                  <code className="hidden truncate text-xs text-slate-500 md:block">{operation.operationId}</code>
                  <span className="text-center text-slate-400 group-open:rotate-90">›</span>
                </summary>
                <div className="border-t border-slate-100 bg-slate-50 px-4 py-4 text-sm">
                  <div className="min-w-0 space-y-2">
                    <div><span className="text-xs font-bold uppercase text-slate-500">Summary</span><p className="text-slate-800">{operation.summary}</p></div>
                    {operation.description && <div><span className="text-xs font-bold uppercase text-slate-500">Description</span><p className="whitespace-pre-wrap text-slate-800">{operation.description}</p></div>}
                    <div><span className="text-xs font-bold uppercase text-slate-500">Operation ID</span><code className="block truncate text-slate-800">{operation.operationId}</code></div>
                    <div className="flex flex-wrap gap-2">{operation.tags.map((tag) => <span key={tag} className="rounded bg-white px-2 py-1 text-xs font-bold text-slate-600">{tag}</span>)}{operation.secured && <span className="inline-flex items-center gap-1 rounded bg-white px-2 py-1 text-xs font-bold text-slate-600"><ShieldCheck size={13} /> Authenticated</span>}</div>
                  </div>
                  <OperationConsole operation={operation} />
                </div>
              </details>
            ))}
            {loading && <div className="table-state">กำลังโหลด OpenAPI contract</div>}
            {!loading && filtered.length === 0 && <div className="table-state">ไม่พบ endpoint ที่ตรงกับตัวกรอง</div>}
          </section>
        </>
      ) : (
        <section className="overflow-hidden rounded-md border border-slate-200 bg-slate-950">
          <div className="flex items-center justify-between border-b border-slate-700 px-4 py-2 text-xs font-bold text-slate-300">
            <span>platform-api.yaml</span>
            <button className="inline-flex items-center gap-2 rounded px-2 py-1 hover:bg-slate-800" type="button" onClick={() => void copy(source)} disabled={!source}>{copied === source ? <Check size={15} /> : <Copy size={15} />} คัดลอก</button>
          </div>
          <pre className="max-h-[65vh] overflow-auto p-4 text-xs leading-6 text-slate-200">{source || "กำลังโหลด OpenAPI contract"}</pre>
        </section>
      )}
    </div>
  );
}

function OperationConsole({ operation }: { operation: Operation }) {
  const [values, setValues] = useState<Record<string, string>>({});
  const [body, setBody] = useState(() => {
    if (!operation.requestBody) return "";
    const example = requestBodyExample(operation.requestBody);
    return example === undefined ? "{}" : JSON.stringify(example, null, 2);
  });
  const [middlewareKey, setMiddlewareKey] = useState("");
  const [pending, setPending] = useState(false);
  const [result, setResult] = useState<{ status: number; statusText: string; body: string } | null>(null);
  const [error, setError] = useState("");

  async function execute() {
    if (operation.method !== "get" && !window.confirm(`ส่ง ${operation.method.toUpperCase()} request จริงไปยัง ${operation.path}?`)) return;
    setPending(true);
    setError("");
    setResult(null);
    try {
      let path = operation.path;
      const query = new URLSearchParams();
      const headers: Record<string, string> = {};
      for (const parameter of operation.parameters) {
        if (!parameter.name || !parameter.in) continue;
        const value = values[`${parameter.in}:${parameter.name}`]?.trim() ?? "";
        if (parameter.required && !value) throw new Error(`กรุณาระบุ ${parameter.name}`);
        if (!value) continue;
        if (parameter.in === "path") path = path.replaceAll(`{${parameter.name}}`, encodeURIComponent(value));
        if (parameter.in === "query") query.set(parameter.name, value);
        if (parameter.in === "header") headers[parameter.name] = value;
      }
      if (operation.securityNames.includes("csrfToken")) headers["X-CSRF-Token"] = csrfToken();
      if (operation.securityNames.includes("middlewareApiKey") && middlewareKey) headers["X-Api-Key"] = middlewareKey;
      const response = await api(`${path}${query.size ? `?${query}` : ""}`, {
        method: operation.method.toUpperCase(),
        headers,
        ...(operation.requestBody ? { body } : {}),
      });
      setResult({ status: response.status, statusText: response.statusText, body: await response.text() });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เรียก API ไม่สำเร็จ");
    } finally {
      setPending(false);
    }
  }

  return <div className="mt-4 grid gap-4 border-t border-slate-200 pt-4 lg:grid-cols-2">
    <section className="min-w-0 space-y-3">
      <h4 className="font-extrabold text-slate-900">API specification</h4>
      {operation.parameters.length > 0 && <div><span className="text-xs font-bold uppercase text-slate-500">Parameters</span><div className="mt-1 overflow-auto rounded border border-slate-200 bg-white">{operation.parameters.map((parameter) => <div key={`${parameter.in}:${parameter.name}`} className="grid grid-cols-[minmax(120px,1fr)_80px_2fr] gap-2 border-b border-slate-100 px-3 py-2 last:border-0"><code>{parameter.name}</code><span className="text-xs font-bold uppercase text-slate-500">{parameter.in}</span><span className="text-xs text-slate-600">{parameter.required ? "required · " : ""}{parameter.description ?? ""}</span></div>)}</div></div>}
      {operation.requestBody != null && <SpecBlock title="Request body" value={operation.requestBody} />}
      {operation.responses != null && <SpecBlock title="Responses" value={operation.responses} />}
    </section>
    <section className="min-w-0 space-y-3 rounded-md border border-slate-200 bg-white p-4">
      <div><h4 className="font-extrabold text-slate-900">Try it</h4><p className="text-xs text-slate-500">ส่ง request ไปยัง Gateway ปัจจุบันด้วย session ของผู้ใช้</p></div>
      {operation.parameters.map((parameter) => parameter.name && parameter.in ? <label key={`${parameter.in}:${parameter.name}`} className="block text-xs font-bold text-slate-600">{parameter.name} <span className="font-normal">({parameter.in}{parameter.required ? ", required" : ""})</span><input className={`${inputClass} mt-1`} value={values[`${parameter.in}:${parameter.name}`] ?? ""} onChange={(event) => setValues((current) => ({ ...current, [`${parameter.in}:${parameter.name}`]: event.target.value }))} /></label> : null)}
      {operation.securityNames.includes("middlewareApiKey") && <label className="block text-xs font-bold text-slate-600">Middleware API Key<input className={`${inputClass} mt-1`} type="password" autoComplete="off" value={middlewareKey} onChange={(event) => setMiddlewareKey(event.target.value)} placeholder="ใช้เฉพาะ request นี้และไม่บันทึก" /></label>}
      {operation.requestBody != null && <label className="block text-xs font-bold text-slate-600">JSON body<textarea className="mt-1 min-h-36 w-full rounded-md border border-slate-300 bg-slate-950 p-3 font-mono text-xs text-slate-100" value={body} onChange={(event) => setBody(event.target.value)} spellCheck={false} /></label>}
      <button className={primaryButtonClass} type="button" onClick={() => void execute()} disabled={pending}>{pending ? "กำลังส่ง..." : "Execute"}</button>
      {error && <p className="rounded bg-rose-50 px-3 py-2 text-xs font-bold text-rose-700">{error}</p>}
      {result && <div className="overflow-hidden rounded border border-slate-200"><div className="bg-slate-100 px-3 py-2 text-xs font-bold">HTTP {result.status} {result.statusText}</div><pre className="max-h-72 overflow-auto whitespace-pre-wrap bg-slate-950 p-3 text-xs text-slate-100">{result.body || "(empty response)"}</pre></div>}
    </section>
  </div>;
}

function SpecBlock({ title, value }: { title: string; value: unknown }) {
  return <div><span className="text-xs font-bold uppercase text-slate-500">{title}</span><pre className="mt-1 max-h-52 overflow-auto rounded border border-slate-200 bg-white p-3 text-xs text-slate-700">{JSON.stringify(value, null, 2)}</pre></div>;
}

function requestBodyExample(requestBody: unknown) {
  if (!requestBody || typeof requestBody !== "object") return undefined;
  const content = (requestBody as { content?: Record<string, { example?: unknown; examples?: Record<string, { value?: unknown }>; schema?: { example?: unknown } }> }).content;
  const json = content?.["application/json"];
  if (!json) return undefined;
  if (json.example !== undefined) return json.example;
  const named = Object.values(json.examples ?? {}).find((example) => example.value !== undefined);
  return named?.value ?? json.schema?.example;
}
