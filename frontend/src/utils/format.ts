// 数值与路径的展示格式化 / Display formatting for numbers and paths

const ByteUnits = ["B", "KiB", "MiB", "GiB", "TiB"];

/** formatBytes 以二进制单位格式化字节数 */
export function formatBytes(bytes: number | undefined | null): string {
    if (bytes === undefined || bytes === null || !Number.isFinite(bytes)) return "-";
    if (bytes < 1024) return `${bytes} ${ByteUnits[0]}`;
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < ByteUnits.length - 1) {
        value /= 1024;
        unit++;
    }
    return `${value.toFixed(value >= 100 ? 0 : value >= 10 ? 1 : 2)} ${ByteUnits[unit]}`;
}

/** formatNumber 按千位分隔展示整数 */
export function formatNumber(value: number | undefined | null): string {
    if (value === undefined || value === null || !Number.isFinite(value)) return "-";
    return value.toLocaleString("en-US");
}

/** formatHex 以 0x 前缀展示无符号整数 */
export function formatHex(value: number | undefined | null): string {
    if (value === undefined || value === null || !Number.isFinite(value)) return "-";
    return `0x${(value >>> 0).toString(16)}`;
}

/** baseName 取路径的最后一段，兼容正反斜杠 */
export function baseName(path: string): string {
    const normalized = path.replace(/\\/g, "/").replace(/\/+$/, "");
    const index = normalized.lastIndexOf("/");
    return index >= 0 ? normalized.slice(index + 1) : normalized;
}

/** dirName 取路径的父目录，兼容正反斜杠 */
export function dirName(path: string): string {
    const normalized = path.replace(/\\/g, "/").replace(/\/+$/, "");
    const index = normalized.lastIndexOf("/");
    return index > 0 ? normalized.slice(0, index) : normalized;
}

/** joinPath 用当前路径的分隔符风格拼接一段子路径 */
export function joinPath(base: string, child: string): string {
    const separator = base.includes("\\") && !base.includes("/") ? "\\" : "/";
    const trimmed = base.replace(/[\\/]+$/, "");
    return `${trimmed}${separator}${child}`;
}

/** percent 返回压缩率之类的百分比文本，分母为零时返回连字符 */
export function percent(numerator: number, denominator: number): string {
    if (!denominator) return "-";
    return `${((numerator / denominator) * 100).toFixed(1)}%`;
}
