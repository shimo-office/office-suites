#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""shimo-net CLI（Python 版，零第三方依赖）

石墨 shimo.net（含客户私有部署）命令行，15 个场景。不依赖 Node.js。
登录：Chrome 专用窗口 -> macOS Keychain。
"""
import sys, os, json, re, socket, ssl, struct, base64, hashlib, subprocess, tempfile, zipfile, time, shutil
import urllib.request, urllib.error, urllib.parse
import xml.etree.ElementTree as ET
from concurrent.futures import ThreadPoolExecutor

VERSION = "0.7.0"

HOME = os.path.join(os.path.expanduser("~"), ".shimo-net-cli")
CONFIG_FILE = os.path.join(HOME, "config.json")
CRED_FILE = os.path.join(HOME, "credentials.json")
FAV_FILE = os.path.join(HOME, "favorites.json")
PROFILE = os.path.join(HOME, "chrome-profile")
SERVICE = "shimo-net-cli"


def normalize_origin(value):
    s = str(value or "").strip()
    if not s:
        raise ValueError("产品网址不能为空")
    if not re.match(r"^https?://", s, re.I):
        s = "https://" + s
    p = urllib.parse.urlparse(s)
    if not p.netloc:
        raise ValueError("产品网址不合法：%s" % value)
    path = p.path.rstrip("/")
    return (p.scheme.lower() + "://" + p.netloc + path).rstrip("/")


def read_config():
    try:
        with open(CONFIG_FILE, encoding="utf-8") as f:
            return json.load(f) or {}
    except Exception:
        return {}


def save_config(origin, **extra):
    os.makedirs(HOME, mode=0o700, exist_ok=True)
    data = read_config()
    data.update({"origin": normalize_origin(origin), "savedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())})
    data.update(extra)
    with open(CONFIG_FILE, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
    try:
        os.chmod(HOME, 0o700); os.chmod(CONFIG_FILE, 0o600)
    except Exception:
        pass
    return data


def resolve_origin():
    env = os.environ.get("SHIMO_NET_ORIGIN")
    if env:
        return normalize_origin(env), "env"
    cfg = read_config().get("origin")
    if cfg:
        return normalize_origin(cfg), "config"
    return "https://shimo.net", "default"


ORIGIN, ORIGIN_SOURCE = resolve_origin()
API = ORIGIN + "/api/v1"


def set_origin(value):
    global ORIGIN, API, ORIGIN_SOURCE
    ORIGIN = normalize_origin(value)
    API = ORIGIN + "/api/v1"
    ORIGIN_SOURCE = "config"
    save_config(ORIGIN)
    return ORIGIN


def ensure_origin(interactive=True):
    global ORIGIN, API, ORIGIN_SOURCE
    if os.environ.get("SHIMO_NET_ORIGIN"):
        ORIGIN, ORIGIN_SOURCE = resolve_origin()
        API = ORIGIN + "/api/v1"
        return ORIGIN
    if read_config().get("origin"):
        ORIGIN, ORIGIN_SOURCE = resolve_origin()
        API = ORIGIN + "/api/v1"
        return ORIGIN
    if interactive and sys.stdin.isatty() and not JSON_OUT:
        value = input("首次使用，请输入产品网址（默认 https://shimo.net）：").strip()
        return set_origin(value or "https://shimo.net")
    return ORIGIN

ARGS = sys.argv[1:]
CMD = ARGS[0] if ARGS else "help"
JSON_OUT = "--json" in ARGS
RELOGIN = "--relogin" in ARGS
POS = [a for a in ARGS[1:] if not a.startswith("--")]

def opt(name, default=None):
    if name in ARGS:
        i = ARGS.index(name)
        return ARGS[i + 1] if i + 1 < len(ARGS) else default
    return default

def has(name):
    return name in ARGS

def out(obj):
    if JSON_OUT:
        print(json.dumps(obj, ensure_ascii=False, indent=2))
    return obj

def fail(msg, code=1):
    if JSON_OUT:
        print(json.dumps({"ok": False, "error": msg}, ensure_ascii=False), file=sys.stderr)
    else:
        print("错误：" + msg, file=sys.stderr)
    sys.exit(code)

# ---------- 工具 ----------

def fmt_time(ts):
    if not ts:
        return ""
    try:
        if isinstance(ts, (int, float)):
            n = ts if ts < 1e12 else ts / 1000.0
            return time.strftime("%Y-%m-%d", time.localtime(n))
        s = str(ts)
        m = re.match(r"^(\d{4}-\d{2}-\d{2})", s)
        if m:
            return m.group(1)
        return s[:10]
    except Exception:
        return ""

def time_of(ts):
    if not ts:
        return 0
    if isinstance(ts, (int, float)):
        return ts * 1000 if ts < 1e12 else ts
    try:
        return time.mktime(time.strptime(str(ts)[:19], "%Y-%m-%dT%H:%M:%S")) * 1000
    except Exception:
        return 0

def parse_since(v):
    if not v:
        return 0
    m = re.match(r"^(\d+)([dhmw])$", str(v), re.I)
    if m:
        n = int(m.group(1)); unit = m.group(2).lower()
        ms = {"d": 86400000, "h": 3600000, "m": 60000, "w": 604800000}[unit]
        return time.time() * 1000 - n * ms
    try:
        return time.mktime(time.strptime(str(v), "%Y-%m-%d")) * 1000
    except Exception:
        raise ValueError("看不懂的时间：" + str(v) + "（示例：7d / 24h / 2026-11-01）")

TYPE_ROUTE = {"document": "docs", "documentPro": "docx", "spreadsheet": "sheets", "presentation": "presentation",
              "table": "tables", "form": "forms", "flowchart": "flowcharts", "mindmap": "mindmaps"}

def guid_from_url(v):
    if not v:
        return ""
    s = str(v).strip()
    m = re.search(r"/(folder|space|docs|docx|sheets|presentation|tables|forms|flowcharts|mindmaps|files)/([A-Za-z0-9]+)", s)
    if m:
        return m.group(2)
    if re.match(r"^[A-Za-z0-9]{8,}$", s):
        return s
    return ""

def url_of(f):
    if f.get("isSpace"):
        return ORIGIN + "/space/" + (f.get("guid") or f.get("space_guid") or "")
    if f.get("isFolder"):
        return ORIGIN + "/folder/" + f.get("guid", "")
    u = f.get("url") or ""
    if u.startswith("/"):
        return ORIGIN + u
    seg = TYPE_ROUTE.get(f.get("type"))
    return ORIGIN + "/" + (seg + "/" if seg else "files/") + f.get("guid", "")

def type_label(f):
    if f.get("isSpace"):
        return "空间"
    if f.get("isFolder") or f.get("type") == "folder":
        return "文件夹"
    return {"document": "文档", "documentPro": "文稿", "spreadsheet": "表格", "presentation": "幻灯片",
            "table": "应用表格", "form": "表单", "flowchart": "流程图", "mindmap": "思维导图"}.get(f.get("type")) or ("云文件" if f.get("isCloudFile") else (f.get("type") or "文件"))

def sanitize(name):
    return re.sub(r'[\\/:*?"<>|]', "_", str(name)).lstrip(".").strip()

def ext_from_url(url, fallback="bin"):
    try:
        m = re.search(r"\.([A-Za-z0-9]{1,6})$", urllib.parse.urlparse(url).path)
        if m:
            return m.group(1).lower()
    except Exception:
        pass
    return fallback

SHARE_MODES = {
    "private": "私有（仅自己）",
    "enterprise_readonly": "企业内只读",
    "enterprise_commentable": "企业内可评论",
    "enterprise_editable": "企业内可编辑",
    "readonly": "公开链接 · 只读",
    "commentable": "公开链接 · 可评论",
    "editable": "公开链接 · 可编辑",
}
IMPORT_TYPES = {"md": "document", "doc": "documentPro", "docx": "documentPro",
                "xls": "spreadsheet", "xlsx": "spreadsheet",
                "ppt": "presentation", "pptx": "presentation"}
CREATE_TYPES = {"doc": "document", "document": "document", "sheet": "spreadsheet", "spreadsheet": "spreadsheet",
                "slide": "presentation", "presentation": "presentation", "table": "table",
                "form": "form", "mindmap": "mindmap", "flowchart": "flowchart"}

# ---------- Keychain / 凭据 ----------

def keychain_available():
    if sys.platform != "darwin":
        return False
    try:
        subprocess.run(["security", "-h"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
        return True
    except Exception:
        return False

def kc_save(account, cookie):
    subprocess.run(["security", "add-generic-password", "-U", "-s", SERVICE, "-a", str(account or "default"), "-w", cookie],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)

def kc_load():
    try:
        r = subprocess.run(["security", "find-generic-password", "-s", SERVICE, "-w"],
                           capture_output=True, text=True, check=True)
        return r.stdout.strip()
    except Exception:
        return ""

def kc_clear():
    try:
        subprocess.run(["security", "delete-generic-password", "-s", SERVICE],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    except Exception:
        pass

def cred_source():
    if os.environ.get("SHIMO_NET_COOKIE"):
        return "env"
    if keychain_available() and kc_load():
        return "keychain"
    try:
        with open(CRED_FILE, encoding="utf-8") as f:
            if json.load(f).get("cookie"):
                return "file"
    except Exception:
        pass
    return None

def read_cred():
    if os.environ.get("SHIMO_NET_COOKIE"):
        return {"origin": ORIGIN, "cookie": os.environ["SHIMO_NET_COOKIE"], "account": "(env)"}
    meta = None
    try:
        with open(CRED_FILE, encoding="utf-8") as f:
            meta = json.load(f)
    except Exception:
        pass
    if keychain_available():
        cookie = kc_load()
        if cookie:
            return {"origin": ORIGIN, "cookie": cookie, "account": (meta or {}).get("account") or "(keychain)"}
    if meta and meta.get("cookie"):
        return meta
    return None

def write_cred(cred):
    # 不在磁盘记录任何账号信息：Keychain 只给通用标签，文件里连账号字段都不写
    os.makedirs(HOME, mode=0o700, exist_ok=True)
    in_keychain = False
    if keychain_available():
        try:
            kc_save("shimo-net", cred["cookie"])
            in_keychain = True
        except Exception:
            pass
    if in_keychain:
        on_disk = {"origin": cred.get("origin"), "savedAt": cred.get("savedAt"), "storage": "keychain"}
    else:
        on_disk = {"origin": cred.get("origin"), "cookie": cred["cookie"], "savedAt": cred.get("savedAt")}
    with open(CRED_FILE, "w", encoding="utf-8") as f:
        json.dump(on_disk, f, ensure_ascii=False, indent=2)
    try:
        os.chmod(HOME, 0o700); os.chmod(CRED_FILE, 0o600)
    except Exception:
        pass
    return {"file": CRED_FILE, "keychain": in_keychain}

def clear_cred():
    try:
        os.unlink(CRED_FILE)
    except Exception:
        pass
    if keychain_available():
        kc_clear()

def require_cookie():
    c = read_cred()
    if not c or not c.get("cookie"):
        raise RuntimeError("未登录：请先运行 shimo-net login")
    return c["cookie"]

# ---------- 常用位置（别名） ----------

def load_fav():
    try:
        with open(FAV_FILE, encoding="utf-8") as f:
            return json.load(f).get("aliases", {})
    except Exception:
        return {}

def save_fav(aliases):
    os.makedirs(HOME, mode=0o700, exist_ok=True)
    with open(FAV_FILE, "w", encoding="utf-8") as f:
        json.dump({"aliases": aliases}, f, ensure_ascii=False, indent=2)

def fav_name(spec):
    if spec is None:
        return ""
    s = str(spec).strip()
    if s.startswith("@"):
        s = s[1:]
    return s

def resolve_target(spec):
    """把 desktop / @别名 / 链接 / guid / 空间名 解析成 guid 或 Desktop"""
    if not spec:
        return "Desktop"
    s = str(spec).strip()
    if s.lower() in ("desktop", "桌面", "我的桌面", "我的桌面"):
        return "Desktop"
    alias = fav_name(s)
    aliases = load_fav()
    if alias in aliases:
        return guid_from_url(aliases[alias].get("url")) or aliases[alias].get("url")
    g = guid_from_url(s)
    if g:
        return g
    # 空间/文件夹名字兜底：精确匹配空间名
    try:
        for sp in list_spaces():
            if sp["name"] == s:
                return sp["guid"]
    except Exception:
        pass
    raise ValueError("认不出这个位置：%s（可传链接、guid、desktop 或 @别名；用 `shimo-net fav list` 看已有别名）" % s)

def describe_target(guid):
    if not guid or guid == "Desktop":
        return {"guid": "Desktop", "name": "我的桌面", "kind": "desktop"}
    try:
        info = file_info(guid)
        return {"guid": guid, "name": info.get("name"), "kind": "space" if info.get("isSpace") else ("folder" if info.get("isFolder") else info.get("type")), "url": url_of({**info, "guid": guid})}
    except Exception:
        return {"guid": guid, "name": None, "kind": "unknown"}

# ---------- HTTP ----------

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None

OPENER = urllib.request.build_opener(NoRedirect)

def header_get(headers, name):
    for k, v in headers.items():
        if k.lower() == name.lower():
            return v
    return None

def req(path, method="GET", body=None, raw=False, headers=None, cookie=None, absolute=False):
    url = path if absolute else API + path
    c = cookie or require_cookie()
    h = {"cookie": c, "referer": ORIGIN + "/", "x-requested-with": "XMLHttpRequest"}
    if headers:
        h.update(headers)
    data = None
    if body is not None:
        if raw:
            data = body
        else:
            data = json.dumps(body).encode("utf-8")
            h["content-type"] = "application/json"
    r = urllib.request.Request(url, data=data, method=method, headers=h)
    try:
        resp = OPENER.open(r, timeout=90)
        status, hdrs, payload = resp.status, dict(resp.headers), resp.read()
    except urllib.error.HTTPError as e:
        status, hdrs, payload = e.code, dict(e.headers), e.read()
    text = payload.decode("utf-8", "replace")
    try:
        js = json.loads(text)
    except Exception:
        js = text
    return {"status": status, "headers": hdrs, "data": js}

def fetch_json(url, cookie=None):
    h = {}
    if cookie:
        h["cookie"] = cookie
    r = urllib.request.Request(url, headers=h)
    with urllib.request.urlopen(r, timeout=60) as resp:
        return json.loads(resp.read().decode("utf-8"))

# ---------- API ----------

def users_me(cookie=None):
    r = req("/users/me", cookie=cookie)
    if r["status"] != 200:
        raise RuntimeError("登录已过期或不可用（HTTP %s）" % r["status"])
    return r["data"]

def whoami():
    return users_me()

def search_files(keyword, limit=20, pages=3):
    out, body = [], {"keyword": keyword}
    for _ in range(pages):
        r = req("/search/files", method="POST", body=body)
        if r["status"] != 200:
            raise RuntimeError("搜索失败：HTTP %s %s" % (r["status"], str(r["data"])[:200]))
        for item in r["data"].get("results", []):
            s = item.get("source") or {}
            is_folder = bool(s.get("isFolder")) or s.get("type") == "folder"
            out.append({
                "guid": s.get("guid"), "name": s.get("name"), "type": s.get("type"),
                "isFolder": is_folder, "isSpace": bool(s.get("isSpace")),
                "shareMode": s.get("shareMode"), "updatedAt": s.get("updatedAt"),
                "path": " / ".join([a.get("name") for a in (s.get("ancestors") or []) if a.get("name")]),
                "url": url_of({**s, "isFolder": is_folder}),
            })
            if len(out) >= limit:
                return out
        nxt = r["data"].get("next")
        if not nxt:
            break
        q = urllib.parse.parse_qs(nxt)
        body = {"keyword": q.get("keyword", [keyword])[0]}
        if q.get("fileType"):
            body["fileType"] = q["fileType"][0]
        if q.get("offset"):
            body["offset"] = int(q["offset"][0])
        if q.get("size"):
            body["size"] = int(q["size"][0])
        if q.get("lastTimestamp"):
            body["lastTimestamp"] = int(q["lastTimestamp"][0])
    return out

def list_folder(target, limit=200, pages=10):
    guid = resolve_target(target) if target else "Desktop"
    if guid == "Desktop":
        guid = "Desktop"
    url = "/files?folder=" + urllib.parse.quote(str(guid))
    out = []
    for _ in range(pages):
        r = req(url)
        if r["status"] != 200:
            raise RuntimeError("读取文件夹失败：HTTP %s %s" % (r["status"], str(r["data"])[:200]))
        j = r["data"] or {}
        for f in j.get("list") or []:
            is_folder = bool(f.get("isFolder"))
            out.append({
                "guid": f.get("guid"), "name": f.get("name"), "type": f.get("type"),
                "isFolder": is_folder, "isSpace": bool(f.get("isSpace")), "isCloudFile": bool(f.get("isCloudFile")),
                "fileSize": f.get("fileSize"), "updatedAt": f.get("updatedAt"),
                "shareMode": f.get("shareMode"),
                "url": url_of({**f, "isFolder": is_folder}),
            })
            if len(out) >= limit:
                return out
        nxt = j.get("nextURL")
        if isinstance(nxt, str) and nxt:
            url = nxt.replace(ORIGIN, "")
            if not url.startswith("/"):
                url = "/" + url
        elif isinstance(nxt, dict) and (nxt.get("url") or nxt.get("nextURL")):
            url = str(nxt.get("url") or nxt.get("nextURL")).replace(ORIGIN, "")
        else:
            break
    return out

def file_info(guid):
    r = req("/files/" + guid)
    if r["status"] != 200:
        raise RuntimeError("读取文件信息失败：HTTP %s" % r["status"])
    return r["data"]

def ancestors(guid):
    r = req("/files/%s/ancestors" % guid)
    if r["status"] != 200:
        raise RuntimeError("读取所在路径失败：HTTP %s" % r["status"])
    return [{"guid": a.get("guid"), "name": a.get("name"), "isFolder": bool(a.get("isFolder")), "isSpace": bool(a.get("isSpace")),
             "url": (ORIGIN + a["url"]) if str(a.get("url") or "").startswith("/") else url_of({**a, "isFolder": True})}
            for a in (r["data"].get("ancestors") or [])]

def list_spaces(limit=300):
    out, url = [], "/files/spaces"
    for _ in range(10):
        r = req(url)
        if r["status"] != 200:
            raise RuntimeError("读取空间列表失败：HTTP %s" % r["status"])
        j = r["data"] or {}
        for s in j.get("spaces") or []:
            out.append({"guid": s.get("guid"), "name": s.get("name"), "url": ORIGIN + "/space/" + s.get("guid", ""),
                        "isSpace": True, "role": s.get("role"), "updatedAt": s.get("updatedAt"), "fileSize": s.get("fileSize")})
            if len(out) >= limit:
                return out
        nxt = j.get("next")
        if not nxt:
            break
        q = str(nxt)
        url = "/files/spaces" + (q if q.startswith("?") else "?" + q)
    return out

def upload_file(local_path, parent_guid):
    with open(local_path, "rb") as f:
        buf = f.read()
    name = os.path.basename(local_path)
    ext = os.path.splitext(name)[1][1:].lower()
    r = req("/files/upload?encodedFileName=" + urllib.parse.quote(name), method="POST", body=buf, raw=True,
            headers={"content-type": "application/octet-stream", "x-file-parent-guid": parent_guid or "Desktop",
                     "x-file-size": str(len(buf)), "x-file-type": ext})
    if r["status"] != 200:
        raise RuntimeError("上传失败：HTTP %s %s" % (r["status"], str(r["data"])[:200]))
    return r["data"]

def import_file(cloud_guid, typ):
    r = req("/files/import/" + cloud_guid, method="POST", body={"type": typ})
    tid = ((r["data"] or {}).get("data") or {}).get("taskId")
    if r["status"] != 200 or not tid:
        raise RuntimeError("导入失败：HTTP %s %s" % (r["status"], str(r["data"])[:200]))
    return tid

def import_progress(task_id, timeout=120):
    deadline = time.time() + timeout
    while time.time() < deadline:
        r = req("/files/import/progress", method="POST", body={"taskId": task_id})
        d = ((r["data"] or {}).get("data") or {})
        if d.get("progress", 0) >= 100:
            return d
        time.sleep(0.7)
    raise RuntimeError("导入超时")

def set_share_mode(guid, mode):
    if mode not in SHARE_MODES:
        raise ValueError("不支持的权限：%s，可选：%s" % (mode, ", ".join(SHARE_MODES)))
    r = req("/files/" + guid, method="PATCH", body={"shareMode": mode})
    if r["status"] != 200:
        raise RuntimeError("设置权限失败：HTTP %s %s" % (r["status"], str(r["data"])[:200]))
    if (r["data"] or {}).get("shareMode") == "unknown_share_mode":
        raise RuntimeError("服务端拒绝了这个权限值")
    return r["data"].get("shareMode")

def rename_file(guid, name):
    r = req("/files/" + guid, method="PATCH", body={"name": name})
    if r["status"] != 200:
        raise RuntimeError("重命名失败：HTTP %s" % r["status"])
    return r["data"]

def create_file(parent_guid, typ, name):
    shimo_type = CREATE_TYPES.get(typ)
    if not shimo_type:
        raise ValueError("不支持的新建类型：%s（可用：%s）" % (typ, ", ".join(CREATE_TYPES)))
    r = req("/files/create/%s?parentGuid=%s&name=%s" % (shimo_type, urllib.parse.quote(parent_guid or "Desktop"), urllib.parse.quote(name)))
    if r["status"] != 302:
        raise RuntimeError("新建失败：HTTP %s %s" % (r["status"], str(r["data"])[:200]))
    loc = header_get(r["headers"], "location") or ""
    m = re.search(r"/(docs|docx|sheets|presentation|tables|forms|flowcharts|mindmaps)/([A-Za-z0-9]+)", loc)
    if not m:
        raise RuntimeError("新建成功但没解析到 guid：" + loc)
    guid = m.group(2)
    info = file_info(guid)
    return {**info, "guid": guid, "url": url_of({**info, "guid": guid})}

def delete_file(guid):
    r = req("/files/" + guid, method="DELETE")
    if r["status"] not in (200, 204):
        raise RuntimeError("删除失败：HTTP %s" % r["status"])
    return True

def list_trash():
    r = req("/files/trashes")
    if r["status"] != 200:
        raise RuntimeError("读取回收站失败：HTTP %s" % r["status"])
    arr = r["data"] if isinstance(r["data"], list) else (r["data"] or {}).get("list", [])
    return [{"guid": f.get("guid"), "name": f.get("name"), "type": f.get("type"),
             "url": (ORIGIN + f["url"]) if str(f.get("url") or "").startswith("/") else url_of(f),
             "deletedAt": f.get("deletedAt") or f.get("updatedAt")} for f in arr]

def restore_file(guid):
    r = req("/files/trashes/" + guid, method="PATCH")
    if r["status"] != 200:
        raise RuntimeError("恢复失败：HTTP %s %s" % (r["status"], str(r["data"])[:200]))
    return True

def purge_file(guid):
    # 已禁用：彻底删除只能到石墨网页手动操作
    raise RuntimeError("彻底删除不在 CLI 支持范围：请到石墨网页的「回收站」里手动删除")

def export_file(guid, prefer):
    alts = {"document": ["md", "docx", "pdf"], "documentPro": ["docx", "pdf"], "spreadsheet": ["xlsx", "pdf"], "presentation": ["pptx", "pdf"]}
    last = ""
    for typ in [prefer] + [t for t in alts.get(prefer, []) if t != prefer]:
        r = req("/files/export/" + guid, method="POST", body={"type": typ})
        tid = ((r["data"] or {}).get("data") or {}).get("taskId")
        if r["status"] != 200 or not tid:
            last = str(r["data"])[:120]
            continue
        deadline = time.time() + 90
        while time.time() < deadline:
            time.sleep(0.8)
            p = req("/files/export/progress", method="POST", body={"taskId": tid})
            d = ((p["data"] or {}).get("data") or {})
            if d.get("progress", 0) >= 100 and d.get("downloadUrl"):
                return {"type": typ, "url": d["downloadUrl"]}
        last = "导出超时"
    raise RuntimeError("导出失败：" + last)

def download_url(guid):
    r = req("/files/%s/download?requestResourceURL=true" % guid)
    if r["status"] == 200 and (r["data"] or {}).get("resourceURL"):
        return r["data"]["resourceURL"]
    r2 = req("/files/%s/download" % guid)
    loc = header_get(r2["headers"], "location")
    if loc:
        return loc
    raise RuntimeError("获取下载地址失败：HTTP %s" % r["status"])

def fetch_to_file(url, dest):
    os.makedirs(os.path.dirname(dest), exist_ok=True)
    with urllib.request.urlopen(url, timeout=120) as resp:
        data = resp.read()
    with open(dest, "wb") as f:
        f.write(data)
    return len(data)

def audit_permissions(target, limit=200, concurrency=4):
    files = [f for f in list_folder(target, limit=limit) if not f["isFolder"] and not f["isSpace"]]
    results = []
    def one(f):
        try:
            info = file_info(f["guid"])
            return {"guid": f["guid"], "name": info.get("name") or f["name"], "type": info.get("type"),
                    "url": url_of({**info, "guid": f["guid"]}), "shareMode": info.get("shareMode")}
        except Exception as e:
            return {"guid": f["guid"], "name": f["name"], "type": f["type"], "url": f["url"], "shareMode": "unknown", "error": str(e)}
    with ThreadPoolExecutor(max_workers=max(1, min(concurrency, len(files) or 1))) as ex:
        results = list(ex.map(one, files))
    results.sort(key=lambda r: r["name"] or "")
    groups = {"public": [], "enterprise": [], "private": []}
    for r in results:
        mode = r.get("shareMode") or ""
        if mode in ("readonly", "commentable", "editable"):
            groups["public"].append(r)
        elif mode.startswith("enterprise"):
            groups["enterprise"].append(r)
        else:
            groups["private"].append(r)
    return {"target": target, "count": len(results), "groups": groups, "results": results}

# ---------- WebSocket（登录用，最小实现） ----------

class WSClient:
    def __init__(self, url, timeout=15):
        self.url = url
        self.timeout = timeout
        self.sock = None

    def connect(self):
        u = urllib.parse.urlparse(self.url.replace("ws://", "http://", 1))
        host, port, path = u.hostname, u.port or 80, u.path or "/"
        self.sock = socket.create_connection((host, port), timeout=self.timeout)
        key = base64.b64encode(os.urandom(16)).decode()
        handshake = ("GET %s HTTP/1.1\r\nHost: %s:%d\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
                     "Sec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n") % (path, host, port, key)
        self.sock.sendall(handshake.encode())
        buf = b""
        while b"\r\n\r\n" not in buf:
            chunk = self.sock.recv(4096)
            if not chunk:
                raise RuntimeError("WebSocket 握手失败：连接关闭")
            buf += chunk
        if b" 101 " not in buf.split(b"\r\n", 1)[0]:
            raise RuntimeError("WebSocket 握手失败：" + buf.split(b"\r\n", 1)[0].decode("utf-8", "replace"))

    def send(self, obj):
        payload = json.dumps(obj).encode("utf-8")
        mask = os.urandom(4)
        header = bytearray([0x81])
        n = len(payload)
        if n < 126:
            header.append(0x80 | n)
        elif n < 65536:
            header.append(0x80 | 126); header += struct.pack("!H", n)
        else:
            header.append(0x80 | 127); header += struct.pack("!Q", n)
        masked = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
        self.sock.sendall(bytes(header) + mask + masked)

    def _read(self, n):
        buf = b""
        while len(buf) < n:
            chunk = self.sock.recv(n - len(buf))
            if not chunk:
                raise RuntimeError("WebSocket 连接关闭")
            buf += chunk
        return buf

    def recv(self):
        data = b""
        while True:
            b1, b2 = self._read(2)
            fin, opcode = b1 & 0x80, b1 & 0x0F
            masked, ln = b2 & 0x80, b2 & 0x7F
            if ln == 126:
                ln = struct.unpack("!H", self._read(2))[0]
            elif ln == 127:
                ln = struct.unpack("!Q", self._read(8))[0]
            mask = self._read(4) if masked else b""
            payload = self._read(ln)
            if masked:
                payload = bytes(c ^ mask[i % 4] for i, c in enumerate(payload))
            if opcode == 9:
                self._pong(payload); continue
            if opcode == 8:
                raise RuntimeError("WebSocket 关闭")
            if opcode in (1, 2):
                data = payload
                if fin:
                    return data.decode("utf-8", "replace")
                continue
            if opcode == 0:
                data += payload
                if fin:
                    return data.decode("utf-8", "replace")

    def _pong(self, payload):
        mask = os.urandom(4)
        header = bytearray([0x8A, 0x80 | len(payload)])
        masked = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
        self.sock.sendall(bytes(header) + mask + masked)

    def call(self, mid, method, params=None, timeout=10):
        self.send({"id": mid, "method": method, "params": params or {}})
        self.sock.settimeout(timeout)
        while True:
            msg = json.loads(self.recv())
            if msg.get("id") == mid:
                if "error" in msg:
                    raise RuntimeError(msg["error"].get("message", "CDP error"))
                return msg.get("result", {})

    def close(self):
        try:
            self.sock.close()
        except Exception:
            pass

CHROME_PATHS = [
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    "/usr/bin/google-chrome",
    "/usr/bin/google-chrome-stable",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser",
    "/snap/bin/chromium",
]

def find_chrome():
    for p in CHROME_PATHS:
        if os.path.exists(p):
            return p
    raise RuntimeError("没找到 Chrome/Chromium/Edge，请先安装 Google Chrome")

def login():
    if opt("--origin"):
        set_origin(opt("--origin"))
    ensure_origin(interactive=True)
    save_config(ORIGIN)
    os.makedirs(PROFILE, mode=0o700, exist_ok=True)
    chrome = find_chrome()
    args = ["--no-first-run", "--no-default-browser-check", "--user-data-dir=" + PROFILE,
            "--remote-debugging-port=0", "--app=%s/login" % ORIGIN]
    if os.environ.get("SHIMO_NET_NO_SANDBOX"):
        args = ["--no-sandbox", "--disable-gpu"] + args
    print("产品网址：%s" % ORIGIN, flush=True)
    print("正在打开 Chrome，请在窗口里用你自己的账号登录 %s…" % ORIGIN, flush=True)
    proc = subprocess.Popen([chrome] + args, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    ws = None
    try:
        port_file = os.path.join(PROFILE, "DevToolsActivePort")
        port = None
        for _ in range(200):
            try:
                with open(port_file, encoding="utf-8") as f:
                    line = f.readline().strip()
                if line:
                    port = int(line); break
            except Exception:
                pass
            time.sleep(0.15)
        if not port:
            raise RuntimeError("Chrome 没有启动调试端口")
        host = urllib.parse.urlparse(ORIGIN).hostname
        target = None
        for _ in range(60):
            try:
                with urllib.request.urlopen("http://127.0.0.1:%d/json" % port, timeout=5) as r:
                    targets = json.loads(r.read().decode("utf-8"))
                for t in targets:
                    if t.get("type") == "page" and host in (t.get("url") or ""):
                        target = t; break
                if target:
                    break
            except Exception:
                pass
            time.sleep(0.5)
        if not target:
            raise RuntimeError("没找到登录页面（可能窗口被关闭）")
        ws = WSClient(target["webSocketDebuggerUrl"])
        ws.connect()
        ws.call(1, "Network.enable")
        deadline = time.time() + 600
        while time.time() < deadline:
            try:
                res = ws.call(2, "Network.getAllCookies", timeout=6)
                cookies = [c for c in res.get("cookies", []) if (c.get("domain") or "").lstrip(".").endswith(host)]
                if cookies:
                    cookie = "; ".join("%s=%s" % (c["name"], c["value"]) for c in cookies)
                    me = users_me(cookie=cookie)
                    write_cred({"origin": ORIGIN, "cookie": cookie, "savedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())})
                    print("登录成功：%s <%s>" % (me.get("name"), me.get("email")), flush=True)
                    return
            except Exception:
                pass
            time.sleep(1.5)
        raise RuntimeError("等待登录超时（10 分钟）")
    finally:
        try:
            ws and ws.close()
        except Exception:
            pass
        try:
            proc.terminate(); proc.wait(timeout=5)
        except Exception:
            try: proc.kill()
            except Exception: pass

def logout():
    clear_cred()
    print("已删除本机凭据（Keychain / 文件）")

# ---------- 读正文 ----------

W = "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}"
A = "{http://schemas.openxmlformats.org/drawingml/2006/main}"
X = "{http://schemas.openxmlformats.org/spreadsheetml/2006/main}"

def extract_docx(path):
    with zipfile.ZipFile(path) as z:
        root = ET.fromstring(z.read("word/document.xml"))
    paras = []
    for p in root.iter(W + "p"):
        texts = [t.text or "" for t in p.iter(W + "t")]
        if texts:
            paras.append("".join(texts))
    return "\n".join(paras)

def extract_pptx(path):
    with zipfile.ZipFile(path) as z:
        names = [n for n in z.namelist() if re.match(r"^ppt/slides/slide\d+\.xml$", n)]
        names.sort(key=lambda n: int(re.search(r"(\d+)", n).group(1)))
        parts = []
        for i, n in enumerate(names, 1):
            root = ET.fromstring(z.read(n))
            texts = [t.text or "" for t in root.iter(A + "t")]
            parts.append("## 幻灯片 %d\n%s" % (i, " ".join(x for x in texts if x)))
    return "\n\n".join(parts)

def extract_xlsx(path):
    with zipfile.ZipFile(path) as z:
        shared = []
        if "xl/sharedStrings.xml" in z.namelist():
            root = ET.fromstring(z.read("xl/sharedStrings.xml"))
            for si in root.iter(X + "si"):
                shared.append("".join(t.text or "" for t in si.iter(X + "t")))
        sheets = sorted([n for n in z.namelist() if re.match(r"^xl/worksheets/sheet\d+\.xml$", n)])
        parts = []
        for i, n in enumerate(sheets, 1):
            root = ET.fromstring(z.read(n))
            rows = []
            for row in root.iter(X + "row"):
                cells = []
                for c in row.iter(X + "c"):
                    v = c.find(X + "v")
                    if v is None:
                        cells.append("")
                        continue
                    val = v.text or ""
                    if c.get("t") == "s":
                        try:
                            val = shared[int(val)]
                        except Exception:
                            pass
                    cells.append(val)
                if any(cells):
                    rows.append("\t".join(cells))
            parts.append("## Sheet %d\n%s" % (i, "\n".join(rows)))
    return "\n\n".join(parts)

def read_text(guid, max_chars=12000):
    info = file_info(guid)
    typ = info.get("type")
    tmp = tempfile.mkdtemp(prefix="shimo-read-")
    try:
        if typ == "document":
            exp = export_file(guid, "md")
            with urllib.request.urlopen(exp["url"], timeout=60) as r:
                text = r.read().decode("utf-8", "replace")
            via = "export md"
        elif typ in ("documentPro", "spreadsheet", "presentation"):
            prefer = {"documentPro": "docx", "spreadsheet": "xlsx", "presentation": "pptx"}[typ]
            exp = export_file(guid, prefer)
            fp = os.path.join(tmp, "file." + prefer)
            fetch_to_file(exp["url"], fp)
            text = {"documentPro": extract_docx, "spreadsheet": extract_xlsx, "presentation": extract_pptx}[typ](fp)
            via = "export " + prefer
        elif info.get("isCloudFile"):
            url = download_url(guid)
            ext = ext_from_url(url, os.path.splitext(info.get("name") or "")[1][1:] or "bin")
            fp = os.path.join(tmp, "file." + ext)
            fetch_to_file(url, fp)
            if ext in ("md", "txt", "csv", "json", "html"):
                with open(fp, encoding="utf-8", errors="replace") as f:
                    text = f.read()
            else:
                raise RuntimeError("云文件 .%s 不能直接读正文，可用 get 下载到本地" % ext)
            via = "download"
        else:
            raise RuntimeError("不支持读取的类型：" + str(typ))
    finally:
        shutil.rmtree(tmp, ignore_errors=True)
    text = text.replace("\r\n", "\n").strip()
    truncated = len(text) > max_chars
    return {"ok": True, "guid": guid, "name": info.get("name"), "type": typ, "typeLabel": type_label(info),
            "url": url_of({**info, "guid": guid}), "via": via, "chars": len(text), "truncated": truncated,
            "text": text[:max_chars]}

# ---------- 输出 ----------

def file_row(f, i):
    badge = "空间" if f.get("isSpace") else ("文件夹" if f.get("isFolder") else type_label(f))
    path = ("  （" + f["path"] + "）") if f.get("path") else ""
    extra = ""
    if f.get("updatedAt"):
        extra += "  更新=" + fmt_time(f["updatedAt"])
    if f.get("shareMode"):
        extra += "  权限=" + f["shareMode"]
    return "%2d. [%s] %s%s\n    %s\n    guid=%s%s" % (i + 1, badge, f.get("name"), path, f.get("url"), f.get("guid"), extra)

# ---------- 命令 ----------

def probe_origin(origin):
    url = normalize_origin(origin) + "/api/v1/users/me"
    req = urllib.request.Request(url, headers={"x-requested-with": "XMLHttpRequest",
                                               "user-agent": "shimo-net-cli/" + VERSION})
    try:
        with urllib.request.urlopen(req, timeout=12) as r:
            return r.status
    except urllib.error.HTTPError as e:
        return e.code
    except Exception as e:
        raise RuntimeError("连不上 %s：%s" % (origin, e))


def cmd_init():
    value = opt("--origin")
    if value:
        set_origin(value)
    else:
        ensure_origin(interactive=True)
        if not read_config().get("origin"):
            set_origin(ORIGIN)
    status = probe_origin(ORIGIN)
    if status not in (200, 401, 403):
        raise RuntimeError("网址可达但接口返回 HTTP %s，请确认是石墨办公套件地址：%s" % (status, ORIGIN))
    print("产品网址：%s" % ORIGIN)
    print("配置已保存：%s" % CONFIG_FILE)
    print("接口可达（HTTP %s）。下一步：shimo-net login" % status)
    if has("--login"):
        login()


def cmd_config():
    sub = POS[0] if POS else "show"
    if sub in ("show", "get", "list"):
        cfg = read_config()
        o = {"origin": ORIGIN, "source": ORIGIN_SOURCE, "configFile": CONFIG_FILE,
             "envOverride": "SHIMO_NET_ORIGIN" if os.environ.get("SHIMO_NET_ORIGIN") else None,
             "savedAt": cfg.get("savedAt")}
        if JSON_OUT:
            return out(o)
        print("产品网址：%s（来源：%s）" % (ORIGIN, ORIGIN_SOURCE))
        print("配置文件：%s" % CONFIG_FILE)
        if o["envOverride"]:
            print("环境变量：SHIMO_NET_ORIGIN=%s（优先级最高）" % ORIGIN)
        return
    if sub in ("set", "use"):
        value = POS[1] if len(POS) > 1 else opt("--origin")
        if not value:
            raise ValueError("用法：shimo-net config set <https://产品域名>")
        set_origin(value)
        status = probe_origin(ORIGIN)
        print("已保存：%s（HTTP %s）" % (ORIGIN, status))
        return
    if sub in ("clear", "reset"):
        cfg = read_config()
        cfg.pop("origin", None)
        os.makedirs(HOME, mode=0o700, exist_ok=True)
        with open(CONFIG_FILE, "w", encoding="utf-8") as f:
            json.dump(cfg, f, ensure_ascii=False, indent=2)
        print("已清除 config.json 里的 origin；如设置了 SHIMO_NET_ORIGIN，仍以环境变量为准")
        return
    raise ValueError("用法：shimo-net config [show|set <url>|clear]")


def cmd_doctor():
    chrome = next((p for p in CHROME_PATHS if os.path.exists(p)), "未找到")
    src = cred_source()
    o = {"version": VERSION, "runtime": "python " + sys.version.split()[0], "chrome": chrome, "home": HOME,
         "origin": ORIGIN, "originSource": ORIGIN_SOURCE, "configFile": CONFIG_FILE if os.path.exists(CONFIG_FILE) else None,
         "credentialsFile": CRED_FILE if os.path.exists(CRED_FILE) else None,
         "storage": {"keychain": "macOS Keychain", "file": "本机文件 0600", "env": "环境变量 SHIMO_NET_COOKIE"}.get(src, "未登录")}
    try:
        me = whoami()
        o["account"] = "%s <%s>（id %s）" % (me.get("name"), me.get("email"), me.get("id"))
    except Exception as e:
        o["account"] = str(e)
    if JSON_OUT:
        return out(o)
    for k in ("version", "runtime", "chrome", "home", "origin", "originSource", "configFile", "credentialsFile", "storage", "account"):
        if o.get(k) is not None:
            print("%s: %s" % (k, o[k]))

def cmd_whoami():
    me = whoami()
    if JSON_OUT:
        out(me)
    else:
        print("%s <%s>（id %s）" % (me.get("name"), me.get("email"), me.get("id")))

def cmd_search():
    if not POS:
        raise ValueError("用法：shimo-net search <关键词> [--limit 20]")
    keyword = POS[0]
    rows = search_files(keyword, limit=int(opt("--limit", 20)))
    if JSON_OUT:
        return out({"keyword": keyword, "count": len(rows), "results": rows})
    print("搜索「%s」：%d 条\n" % (keyword, len(rows)))
    for i, f in enumerate(rows):
        print(file_row(f, i) + "\n")

def cmd_spaces():
    rows = list_spaces(limit=int(opt("--limit", 300)))
    if JSON_OUT:
        return out({"count": len(rows), "spaces": rows})
    print("我参与的空间：%d 个\n" % len(rows))
    for i, s in enumerate(rows):
        print("%3d. %s\n     %s\n     guid=%s%s%s" % (i + 1, s["name"], s["url"], s["guid"],
              "  角色=" + s["role"] if s.get("role") else "", "  更新=" + fmt_time(s.get("updatedAt")) if s.get("updatedAt") else ""))

def cmd_ls():
    if not POS:
        raise ValueError("用法：shimo-net ls <文件夹/空间链接|guid|@别名> [--since 7d] [--limit 200]")
    target = POS[0]
    since = parse_since(opt("--since"))
    rows = list_folder(target, limit=int(opt("--limit", 200)))
    if since:
        rows = [f for f in rows if time_of(f.get("updatedAt")) >= since]
    if JSON_OUT:
        return out({"target": target, "since": opt("--since"), "count": len(rows), "files": rows})
    print("共 %d 项%s：\n" % (len(rows), ("（自 " + fmt_time(since) + "）") if since else ""))
    for i, f in enumerate(rows):
        print(file_row(f, i) + "\n")

def cmd_info():
    if not POS:
        raise ValueError("用法：shimo-net info <文件/文件夹链接|guid|@别名>")
    guid = resolve_target(POS[0])
    if guid == "Desktop":
        return out({"guid": "Desktop", "name": "我的桌面", "kind": "desktop"})
    info = file_info(guid)
    try:
        chain = ancestors(guid)
    except Exception:
        chain = []
    o = {"guid": guid, "name": info.get("name"), "type": info.get("type"), "typeLabel": type_label(info),
         "isFolder": bool(info.get("isFolder")), "isCloudFile": bool(info.get("isCloudFile")),
         "shareMode": info.get("shareMode"), "updatedAt": info.get("updatedAt"), "fileSize": info.get("fileSize"),
         "url": url_of({**info, "guid": guid}), "path": " / ".join(a["name"] for a in chain), "ancestors": chain}
    if JSON_OUT:
        return out(o)
    print("名称：%s" % o["name"])
    print("类型：%s（%s）%s" % (o["typeLabel"], o["type"], " · 云文件" if o["isCloudFile"] else ""))
    print("链接：%s" % o["url"])
    print("权限：%s（%s）" % (o["shareMode"], SHARE_MODES.get(o["shareMode"], "")))
    if o["path"]:
        print("路径：%s" % o["path"])
    if o["updatedAt"]:
        print("更新：%s" % fmt_time(o["updatedAt"]))

def cmd_upload():
    if not POS:
        raise ValueError("用法：shimo-net upload <本地文件> [--to desktop|空间|文件夹|@别名]")
    local = os.path.expanduser(POS[0])
    if not os.path.exists(local):
        raise ValueError("文件不存在：" + local)
    to = opt("--to")
    if not to:
        aliases = load_fav()
        try:
            spaces = list_spaces(limit=10)
        except Exception:
            spaces = []
        lines = ["请指定上传位置："]
        lines.append("  桌面：--to desktop")
        if aliases:
            lines.append("  常用位置（@别名）：" + "、".join("@" + k for k in list(aliases)[:10]))
        if spaces:
            lines.append("  空间（示例）：" + "、".join(s["name"] for s in spaces[:8]))
        lines.append("  文件夹：--to <文件夹链接|guid>")
        raise ValueError("\n".join(lines))
    parent = resolve_target(to)
    data = upload_file(local, parent)
    info = {"ok": True, "guid": data.get("guid"), "name": data.get("name"), "type": data.get("type"),
            "url": url_of({**data, "guid": data.get("guid")}), "parent": describe_target(parent),
            "isCloudFile": bool(data.get("isCloudFile")), "note": "云文件（原样上传，不转换）；要转在线文档请用 import"}
    if JSON_OUT:
        return out(info)
    print("上传完成：%s" % info["name"])
    print("位置：%s" % (info["parent"].get("name") or info["parent"].get("guid")))
    print("链接：%s" % info["url"])
    print("提示：这是原样云文件；要转成在线文档/表格/幻灯片请用 import")

def cmd_import():
    if not POS:
        raise ValueError("用法：shimo-net import <本地文件> --to <桌面|文件夹链接|@别名> [--title 名称] [--share 权限]")
    local = os.path.expanduser(POS[0])
    if not os.path.exists(local):
        raise ValueError("文件不存在：" + local)
    ext = os.path.splitext(local)[1][1:].lower()
    typ = IMPORT_TYPES.get(ext)
    if not typ:
        raise ValueError("暂不支持 .%s —— 目前只支持 md / doc(x) / xls(x) / ppt(x)" % ext)
    to = opt("--to") or "desktop"
    parent = resolve_target(to)
    cloud = upload_file(local, parent)
    tid = import_file(cloud["guid"], typ)
    done = import_progress(tid)
    guid = (done.get("file") or {}).get("guid") or cloud["guid"]
    info = file_info(guid)
    if opt("--title"):
        rename_file(guid, opt("--title")); info["name"] = opt("--title")
    result = {"ok": True, "name": info.get("name"), "guid": guid, "type": info.get("type"),
              "typeLabel": type_label(info), "url": url_of({**info, "guid": guid}),
              "shareMode": info.get("shareMode"), "folder": describe_target(parent)}
    if opt("--share"):
        result["shareMode"] = set_share_mode(guid, opt("--share"))
    if JSON_OUT:
        return out(result)
    print("导入完成：%s" % result["name"])
    print("类型：%s（%s）" % (result["typeLabel"], result["type"]))
    print("链接：%s" % result["url"])
    print("当前权限：%s（%s）" % (result["shareMode"], SHARE_MODES.get(result["shareMode"], result["shareMode"])))
    if not opt("--share"):
        print("\n设置权限：shimo-net share %s <%s>" % (result["url"], "|".join(SHARE_MODES)))

def cmd_share():
    if len(POS) < 2:
        raise ValueError("用法：shimo-net share <链接|guid|@别名> [更多…] <权限：%s>" % "|".join(SHARE_MODES))
    mode = POS[-1]
    if mode not in SHARE_MODES:
        raise ValueError("不支持的权限：%s，可选：%s" % (mode, ", ".join(SHARE_MODES)))
    targets = POS[:-1]
    results = []
    for t in targets:
        try:
            guid = resolve_target(t)
            info = file_info(guid)
            applied = set_share_mode(guid, mode)
            results.append({"ok": True, "guid": guid, "name": info.get("name"), "shareMode": applied,
                            "shareModeLabel": SHARE_MODES.get(applied), "url": url_of({**info, "guid": guid})})
        except Exception as e:
            results.append({"ok": False, "target": t, "error": str(e)})
    ok = sum(1 for r in results if r["ok"])
    payload = {"ok": ok == len(targets), "mode": mode, "modeLabel": SHARE_MODES[mode], "total": len(targets), "changed": ok, "results": results}
    if JSON_OUT:
        return out(payload)
    print("权限设为「%s」：%d/%d 成功" % (payload["modeLabel"], ok, len(targets)))
    for r in results:
        print(("✓ %s  %s" % (r["name"], r["url"])) if r["ok"] else ("✗ %s：%s" % (r["target"], r["error"])))

def cmd_new():
    if not POS:
        raise ValueError("用法：shimo-net new <名称> [--type doc|sheet|slide] [--to <文件夹|@别名>]")
    name = POS[0]
    typ = opt("--type", "doc")
    parent = resolve_target(opt("--to") or "desktop")
    info = create_file(parent, typ, name)
    o = {"ok": True, "name": info.get("name"), "guid": info.get("guid"), "type": info.get("type"),
         "typeLabel": type_label(info), "url": url_of({**info, "guid": info.get("guid")}), "shareMode": info.get("shareMode")}
    if JSON_OUT:
        return out(o)
    print("新建完成：%s（%s）" % (o["name"], o["typeLabel"]))
    print("链接：%s" % o["url"])
    print("当前权限：%s" % o["shareMode"])

def cmd_rename():
    if len(POS) < 2:
        raise ValueError("用法：shimo-net rename <链接|guid|@别名> <新名字>")
    guid = resolve_target(POS[0])
    rename_file(guid, POS[1])
    info = file_info(guid)
    o = {"ok": True, "guid": guid, "name": info.get("name"), "url": url_of({**info, "guid": guid})}
    if JSON_OUT:
        return out(o)
    print("已改名：%s" % o["name"])
    print("链接：%s" % o["url"])

def cmd_delete():
    if not POS:
        raise ValueError("用法：shimo-net delete <链接|guid|@别名>")
    guid = resolve_target(POS[0])
    info = file_info(guid)
    delete_file(guid)
    o = {"ok": True, "guid": guid, "name": info.get("name"), "url": url_of({**info, "guid": guid}),
         "note": "已进回收站，可用 restore 恢复；彻底删除请到石墨网页手动操作"}
    if JSON_OUT:
        return out(o)
    print("已删除（进回收站，可恢复）：%s" % o["name"])
    print("恢复：shimo-net restore %s" % guid)

def cmd_trash():
    rows = list_trash()
    if JSON_OUT:
        return out({"count": len(rows), "files": rows})
    print("回收站：%d 项\n" % len(rows))
    for i, f in enumerate(rows):
        print("%2d. [%s] %s%s\n    guid=%s" % (i + 1, f.get("type"), f.get("name"),
              "  删除=" + fmt_time(f.get("deletedAt")) if f.get("deletedAt") else "", f.get("guid")))

def cmd_restore():
    if not POS:
        raise ValueError("用法：shimo-net restore <文件链接|guid>")
    guid = guid_from_url(POS[0]) or POS[0]
    restore_file(guid)
    try:
        info = file_info(guid)
        o = {"ok": True, "guid": guid, "name": info.get("name"), "url": url_of({**info, "guid": guid})}
    except Exception:
        o = {"ok": True, "guid": guid, "name": None, "url": None}
    if JSON_OUT:
        return out(o)
    print("已恢复%s" % (("：" + o["name"]) if o["name"] else ""))
    if o["url"]:
        print("链接：%s" % o["url"])

def cmd_purge():
    # 按约定：CLI 只允许删除到回收站；彻底删除必须在石墨系统里手动操作
    raise ValueError("彻底删除不在 CLI 支持范围：请到石墨网页的「回收站」里手动删除。CLI 只提供 delete（进回收站）和 restore（恢复）。")

def cmd_audit():
    if not POS:
        raise ValueError("用法：shimo-net audit <文件夹/空间链接|guid|@别名> [--limit 200]")
    r = audit_permissions(POS[0], limit=int(opt("--limit", 200)))
    if JSON_OUT:
        return out(r)
    g = r["groups"]
    print("权限巡检：%d 个文件" % r["count"])
    print("  公开链接：%d   企业内：%d   私有：%d\n" % (len(g["public"]), len(g["enterprise"]), len(g["private"])))
    if g["public"]:
        print("公开可访问（建议收回或确认）：")
        for f in g["public"]:
            print("  ! %s  [%s]  %s" % (f["name"], f["shareMode"], f["url"]))
        print("")
    print("全部：")
    for f in r["results"]:
        print("  %s %s  [%s]" % ("!" if f["shareMode"] in ("readonly", "commentable", "editable") else "·", f["name"], f["shareMode"]))
    if g["public"]:
        print("\n批量收回：shimo-net share %s enterprise_readonly" % " ".join('"%s"' % f["url"] for f in g["public"]))

def cmd_read():
    if not POS:
        raise ValueError("用法：shimo-net read <链接|guid|@别名> [--max-chars 12000]")
    guid = resolve_target(POS[0])
    r = read_text(guid, max_chars=int(opt("--max-chars", 12000)))
    if JSON_OUT:
        return out(r)
    print("《%s》（%s，%d 字%s）\n" % (r["name"], r["typeLabel"], r["chars"], "，截断" if r["truncated"] else ""))
    print(r["text"])
    if r["truncated"]:
        print("\n…（已截断，加 --max-chars 20000 看更多）")

def download_one(f, out_dir):
    if f.get("isFolder") or f.get("isSpace"):
        return {"skipped": True, "name": f.get("name"), "reason": "目录"}
    if f.get("isCloudFile"):
        url = download_url(f["guid"])
        ext = ext_from_url(url, os.path.splitext(f.get("name") or "")[1][1:] or "bin")
        dest = os.path.join(out_dir, sanitize(re.sub(r"\.[A-Za-z0-9]{1,6}$", "", f.get("name") or f["guid"]) + "." + ext))
        n = fetch_to_file(url, dest)
        return {"name": f.get("name"), "type": "cloud", "file": dest, "bytes": n, "url": f.get("url")}
    prefer = {"document": "md", "documentPro": "docx", "spreadsheet": "xlsx", "presentation": "pptx"}.get(f.get("type"), "pdf")
    exp = export_file(f["guid"], prefer)
    ext = ext_from_url(exp["url"], prefer)
    dest = os.path.join(out_dir, sanitize((f.get("name") or f["guid"]) + "." + ext))
    n = fetch_to_file(exp["url"], dest)
    return {"name": f.get("name"), "type": f.get("type"), "exportType": exp["type"], "file": dest, "bytes": n, "url": f.get("url")}

def cmd_pull():
    if not POS:
        raise ValueError("用法：shimo-net pull <文件夹/空间链接|guid|@别名> --out <目录> [--since 7d] [--limit 50]")
    out_dir = os.path.abspath(os.path.expanduser(opt("--out") or "./shimo-pull"))
    os.makedirs(out_dir, exist_ok=True)
    since = parse_since(opt("--since"))
    files = list_folder(POS[0], limit=int(opt("--limit", 50)))
    if since:
        files = [f for f in files if time_of(f.get("updatedAt")) >= since]
    manifest = []
    for f in files:
        try:
            got = download_one(f, out_dir)
            if got.get("skipped"):
                if not JSON_OUT:
                    print("- 跳过目录：%s" % f.get("name"))
            else:
                manifest.append(got)
                if not JSON_OUT:
                    print("✓ %s → %s (%d bytes)" % (f.get("name"), os.path.basename(got["file"]), got["bytes"]))
        except Exception as e:
            if not JSON_OUT:
                print("✗ %s：%s" % (f.get("name"), e))
            manifest.append({"name": f.get("name"), "url": f.get("url"), "error": str(e)})
    with open(os.path.join(out_dir, "manifest.json"), "w", encoding="utf-8") as fp:
        json.dump({"source": POS[0], "dir": out_dir, "since": opt("--since"), "files": manifest}, fp, ensure_ascii=False, indent=2)
    if JSON_OUT:
        return out({"target": POS[0], "dir": out_dir, "files": manifest})
    print("\n已下载到 %s（清单：manifest.json）" % out_dir)

def cmd_get():
    if not POS:
        raise ValueError("用法：shimo-net get <链接|guid|@别名> --out <目录>")
    guid = resolve_target(POS[0])
    info = file_info(guid)
    out_dir = os.path.abspath(os.path.expanduser(opt("--out") or "./shimo-pull"))
    got = download_one({**info, "guid": guid}, out_dir)
    if got.get("skipped"):
        raise ValueError("目标是目录：请用 shimo-net pull <目录> --out <本地目录>")
    if JSON_OUT:
        return out({**got, "url": url_of({**info, "guid": guid})})
    print("已下载：%s（%d bytes）" % (got["file"], got["bytes"]))

def ensure_alias(name):
    if not name:
        raise ValueError("别名不能为空")
    if name.startswith("@"):
        name = name[1:]
    if not re.match(r"^[A-Za-z0-9_\u4e00-\u9fff-]{1,40}$", name):
        raise ValueError("别名只能用中英文、数字、下划线、短横线，最长 40 字")
    return name

def cmd_open():
    if not POS:
        raise ValueError("用法：shimo-net open <链接|guid|@别名>")
    raw = POS[0]
    if str(raw).startswith("http"):
        url = raw
    else:
        guid = resolve_target(raw)
        if guid == "Desktop":
            raise ValueError("桌面没有可打开的链接")
        info = file_info(guid)
        url = url_of({**info, "guid": guid})
    chrome = find_chrome()
    os.makedirs(PROFILE, mode=0o700, exist_ok=True)
    subprocess.Popen([chrome, "--user-data-dir=" + PROFILE, url],
                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    o = {"ok": True, "url": url, "profile": PROFILE, "note": "用已登录的专用 Chrome 配置打开；如未登录会跳到登录页"}
    if JSON_OUT:
        return out(o)
    print("已用登录态浏览器打开：%s" % url)

def cmd_fav():
    sub = POS[0] if POS else "list"
    aliases = load_fav()
    if sub in ("list", "ls"):
        if JSON_OUT:
            return out({"count": len(aliases), "aliases": aliases})
        if not aliases:
            print("还没有常用位置。加一个：shimo-net fav add <别名> <链接> [--note 特征]")
            return
        print("常用位置（%d 个，命令里用 @别名）：\n" % len(aliases))
        for k, v in aliases.items():
            print("@%s  %s\n     %s\n     %s%s" % (k, v.get("name") or "", v.get("url") or "",
                  "特征：" + v["note"] if v.get("note") else "", "  （" + v.get("kind", "") + "）"))
        return
    if sub == "add":
        if len(POS) < 3:
            raise ValueError("用法：shimo-net fav add <别名> <链接|guid> [--note 特征]")
        name = ensure_alias(POS[1])
        guid = resolve_target(POS[2])
        info = file_info(guid)
        aliases[name] = {"name": info.get("name"), "url": url_of({**info, "guid": guid}),
                         "note": opt("--note") or "", "kind": "space" if info.get("isSpace") else ("folder" if info.get("isFolder") else info.get("type")),
                         "savedAt": time.strftime("%Y-%m-%dT%H:%M:%S")}
        save_fav(aliases)
        o = {"ok": True, "alias": "@" + name, **aliases[name]}
        if JSON_OUT:
            return out(o)
        print("已记住 @%s → %s" % (name, aliases[name]["url"]))
        if aliases[name]["note"]:
            print("特征：%s" % aliases[name]["note"])
        print("下次直接：shimo-net ls @%s" % name)
        return
    if sub in ("rm", "remove", "del"):
        if len(POS) < 2:
            raise ValueError("用法：shimo-net fav rm <别名>")
        name = fav_name(POS[1])
        if name not in aliases:
            raise ValueError("没有这个别名：" + name)
        del aliases[name]
        save_fav(aliases)
        if JSON_OUT:
            return out({"ok": True, "removed": "@" + name})
        print("已删除 @%s" % name)
        return
    if sub in ("show", "get"):
        if len(POS) < 2:
            raise ValueError("用法：shimo-net fav show <别名>")
        name = fav_name(POS[1])
        if name not in aliases:
            raise ValueError("没有这个别名：" + name)
        o = {"alias": "@" + name, **aliases[name]}
        if JSON_OUT:
            return out(o)
        print("@%s  %s\n     %s\n     %s%s" % (name, o.get("name"), o.get("url"),
              "特征：" + o["note"] if o.get("note") else "", "  （" + o.get("kind", "") + "）"))
        return
    raise ValueError("用法：shimo-net fav <list|add|rm|show> …")

# ---------- schema / help ----------

def schema():
    return {
        "name": "shimo-net", "version": VERSION, "transport": "cli", "runtime": "python3",
        "auth": {"login": "shimo-net login", "whoami": "shimo-net whoami", "storage": "macOS Keychain", "service": SERVICE, "envOverride": "SHIMO_NET_COOKIE"},
        "origin": {"default": "https://shimo.net", "envOverride": "SHIMO_NET_ORIGIN",
                   "configFile": CONFIG_FILE, "commands": ["init", "config show", "config set <url>"]},
        "favorites": {"file": FAV_FILE, "use": "@别名", "commands": ["fav list", "fav add", "fav rm", "fav show"]},
        "commands": [
            {"name": "init", "usage": "shimo-net init [--origin https://域名] [--login]", "desc": "初始化产品网址并校验接口可达"},
            {"name": "config", "usage": "shimo-net config [show|set <url>|clear]", "desc": "查看/修改产品网址（config.json）"},
            {"name": "search", "usage": "shimo-net search <keyword> [--limit 20]", "desc": "搜文件/文件夹/空间，返回地址"},
            {"name": "spaces", "usage": "shimo-net spaces [--limit 300]", "desc": "列出我参与的空间"},
            {"name": "ls", "usage": "shimo-net ls <链接|guid|@别名> [--since 7d] [--limit 200]", "desc": "列文件夹/空间里的文件"},
            {"name": "info", "usage": "shimo-net info <链接|guid|@别名>", "desc": "文件类型/权限/所在路径"},
            {"name": "upload", "usage": "shimo-net upload <本地文件> [--to desktop|空间|文件夹|@别名]", "desc": "原样上传云文件；不给 --to 会列出可选位置"},
            {"name": "import", "usage": "shimo-net import <本地文件> --to <位置> [--title NAME] [--share MODE]", "desc": "md→文档, doc→文稿, xlsx→表格, pptx→幻灯片"},
            {"name": "new", "usage": "shimo-net new <name> [--type doc|sheet|slide] [--to <位置>]", "desc": "新建空白文件"},
            {"name": "rename", "usage": "shimo-net rename <链接|guid|@别名> <新名字>", "desc": "改名"},
            {"name": "share", "usage": "shimo-net share <链接|guid|@别名> [更多…] <MODE>", "desc": "设置权限（可批量）"},
            {"name": "audit", "usage": "shimo-net audit <文件夹|@别名>", "desc": "权限巡检"},
            {"name": "delete", "usage": "shimo-net delete <链接|guid|@别名>", "desc": "删除到回收站"},
            {"name": "trash", "usage": "shimo-net trash", "desc": "回收站列表"},
            {"name": "restore", "usage": "shimo-net restore <guid>", "desc": "从回收站恢复"},
            {"name": "read", "usage": "shimo-net read <链接|guid|@别名> [--max-chars 12000]", "desc": "读正文（文档 md；文稿/表格/幻灯片本地解析）"},
            {"name": "pull", "usage": "shimo-net pull <链接|@别名> --out <dir> [--since 7d]", "desc": "下载整个文件夹"},
            {"name": "get", "usage": "shimo-net get <链接|guid|@别名> --out <dir>", "desc": "下载单个文件"},
            {"name": "fav", "usage": "shimo-net fav <list|add|rm|show> …", "desc": "常用文件夹别名（下次直接说 @别名）"},
            {"name": "open", "usage": "shimo-net open <链接|guid|@别名>", "desc": "用 CLI 已登录的 Chrome 配置打开链接（免重复登录）"},
            {"name": "whoami", "usage": "shimo-net whoami [--json]", "desc": "当前账号"},
            {"name": "doctor", "usage": "shimo-net doctor [--json]", "desc": "自检（python/chrome/凭据/origin）"},
        ],
        "shareModes": list(SHARE_MODES), "shareModeLabels": SHARE_MODES,
        "importTypes": IMPORT_TYPES, "createTypes": CREATE_TYPES,
        "scenarios": ["搜索", "空间/目录", "文件出处", "导入+权限", "拉取", "新建", "改名", "删除/回收站/恢复",
                      "权限巡检", "批量改权限", "读正文摘要", "周报", "附件下载", "上传（问位置）", "常用文件夹别名"],
        "errors": {"format": "非 0 退出码 + stderr 文本；--json 时 stderr 为 {\"ok\":false,\"error\":\"...\"}"},
    }

HELP = """shimo-net %s —— shimo.net 接口（15 个场景）

用法:
  shimo-net init [--origin URL]                    初始化产品网址并校验可达
  shimo-net config [show|set URL]                  查看 / 修改产品网址
  shimo-net login [--origin URL]                   登录（Chrome → Keychain）
  shimo-net logout                                 退出并删除本机凭据
  shimo-net whoami | doctor                        账号 / 自检
  shimo-net search <关键词> [--limit 20]            1 搜索
  shimo-net spaces [--limit 300]                   2 我的空间清单
  shimo-net ls <链接|@别名> [--since 7d]             2/12 列目录
  shimo-net info <链接|@别名>                       3 文件类型/权限/路径
  shimo-net upload <本地文件> [--to <位置>]          14 上传（不给 --to 会问：桌面/空间/文件夹）
  shimo-net import <本地文件> --to <位置>            4 导入并转在线文档
  shimo-net pull <文件夹|空间> --out <目录>           5 拉取
  shimo-net new <名称> [--type doc|sheet|slide]      6 新建
  shimo-net rename <链接> <新名字>                   7 改名
  shimo-net delete / trash / restore                 8 删除（只进回收站）/ 回收站 / 恢复
  shimo-net audit <文件夹|@别名>                     9 权限巡检
  shimo-net share <链接> [更多…] <权限>              10 设置权限（可批量）
  shimo-net read <链接> [--max-chars N]              11 读正文
  shimo-net get <链接> --out <目录>                  13 下载单个文件
  shimo-net fav <list|add|rm|show> …                15 常用文件夹别名（下次直接 @别名）
  shimo-net open <链接|guid>                        用已登录的浏览器打开（免重复登录）
  shimo-net schema                                  命令/参数/JSON 结构

权限: private | enterprise_readonly | enterprise_commentable | enterprise_editable
      readonly | commentable | editable

全局: --json；SHIMO_NET_ORIGIN 指定私有部署域名；--relogin 遇 401 自动登录后重试
""" % VERSION

DISPATCH = {
    "init": cmd_init, "config": cmd_config,
    "login": lambda: login(), "logout": lambda: logout(), "whoami": cmd_whoami, "doctor": cmd_doctor,
    "search": cmd_search, "spaces": cmd_spaces, "ls": cmd_ls, "info": cmd_info,
    "upload": cmd_upload, "import": cmd_import, "new": cmd_new, "rename": cmd_rename,
    "share": cmd_share, "audit": cmd_audit, "delete": cmd_delete, "trash": cmd_trash,
    "restore": cmd_restore, "purge": cmd_purge, "read": cmd_read, "pull": cmd_pull, "get": cmd_get,
    "fav": cmd_fav, "open": cmd_open,
}

def main():
    if CMD in ("help", "--help", "-h"):
        print(HELP); return
    if CMD == "schema":
        print(json.dumps(schema(), ensure_ascii=False, indent=2)); return
    if CMD in ("version", "--version", "-V"):
        print(json.dumps({"version": VERSION}, ensure_ascii=False) if JSON_OUT else "shimo-net " + VERSION); return
    fn = DISPATCH.get(CMD)
    if not fn:
        print(HELP, file=sys.stderr)
        sys.exit(2)
    try:
        fn()
    except Exception as e:
        msg = str(e)
        if RELOGIN and any(x in msg for x in ("401", "未登录", "登录已过期", "登录不可用")):
            login()
            fn()
        else:
            raise

if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as e:
        fail(str(e))
