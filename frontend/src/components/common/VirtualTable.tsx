import React, {useCallback, useEffect, useMemo, useRef, useState} from "react";
import {Empty, theme} from "antd";
import {CaretDownFilled, CaretUpFilled} from "@ant-design/icons";
import {useVirtualizer} from "@tanstack/react-virtual";
import {useTranslation} from "react-i18next";
import {ColumnWidthsPrefix} from "../../utils/LocalStorageKeys";

/** 排序方向，null 表示回到数据的原始顺序 */
export type SortOrder = "asc" | "desc";

export interface SortState {
    key: string;
    order: SortOrder;
}

/** CSS 自定义属性不在 CSSProperties 的类型里，用交叉类型才能写进 style */
type StyleWithVars = React.CSSProperties & Record<`--${string}`, string>;

/**
 * VirtualColumn 一列的定义
 * width 是这一列的默认轨道尺寸，直接交给 CSS grid，可用 px、fr 或 minmax()；
 * 被用户拖过的列换成固定 px，没拖过的列继续按 fr 分摊剩余宽度
 */
export interface VirtualColumn<T> {
    key: string;
    title: React.ReactNode;
    width: string;
    align?: "left" | "right" | "center";
    /** 用等宽字体渲染，适合 PathID、哈希与偏移 */
    mono?: boolean;
    /** 关掉拖拽手柄，复选框与操作按钮这类固定宽度的列不需要调整，默认可调 */
    resizable?: boolean;
    /**
     * 该列的排序键，给出后表头可点击排序
     * PathID 与哈希是 64 位值，用 bigint 才能正确比较
     */
    sortValue?: (row: T) => string | number | bigint;
    render: (row: T, index: number) => React.ReactNode;
}

export interface VirtualTableProps<T> {
    rows: T[];
    columns: VirtualColumn<T>[];
    rowKey: (row: T, index: number) => string;
    /** 滚动容器高度，默认铺满父容器 */
    height?: number | string;
    rowHeight?: number;
    /** 当前选中行的 key，用于高亮 */
    selectedKey?: string | null;
    onRowClick?: (row: T, index: number) => void;
    onRowDoubleClick?: (row: T, index: number) => void;
    emptyText?: string;
    /** 初始排序列与方向，省略时按传入顺序展示 */
    defaultSort?: SortState;
    /**
     * 列宽记忆用的表标识，给出后用户调整过的列宽会写进 localStorage
     * 省略时列宽只在这个组件存活期间有效
     */
    widthStorageKey?: string;
}

const DefaultRowHeight = 34;
const HeaderHeight = 36;
/** 下限保证列名与排序箭头还放得下，上限避免一列把其他列整个挤出视口后再也找不回来 */
const MinColumnWidth = 48;
const MaxColumnWidth = 1600;
/** 键盘调整列宽的步长，按住 Shift 走大步 */
const KeyboardStep = 8;
const KeyboardLargeStep = 32;
/** grid 轨道写在 CSS 变量里，拖动过程中直接改变量就能完全跳过 React 重渲染 */
const TemplateVariable = "--vt-cols";

/** ColumnWidths 列 key 到像素宽度的映射，只记用户改过的列 */
type ColumnWidths = Record<string, number>;

// 复用一个 Collator：numeric 让 asset_2 排在 asset_10 之前，逐次 new 在上万行上会明显变慢
// One shared Collator: numeric puts asset_2 before asset_10, and constructing it per comparison is measurably slow at scale
const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: "base"});

function clampWidth(width: number): number {
    return Math.min(MaxColumnWidth, Math.max(MinColumnWidth, Math.round(width)));
}

/** loadWidths 读回记忆的列宽，手改过的 localStorage 与旧格式一律当作没有记录 */
function loadWidths(storageKey?: string): ColumnWidths {
    if (!storageKey) return {};
    try {
        const raw = localStorage.getItem(ColumnWidthsPrefix + storageKey);
        if (!raw) return {};
        const parsed: unknown = JSON.parse(raw);
        if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
        const result: ColumnWidths = {};
        Object.entries(parsed as Record<string, unknown>).forEach(([key, value]) => {
            if (typeof value === "number" && Number.isFinite(value)) result[key] = clampWidth(value);
        });
        return result;
    } catch {
        return {};
    }
}

/** saveWidths 写回记忆的列宽，所有列都恢复默认时把整条记录删掉 */
function saveWidths(storageKey: string | undefined, widths: ColumnWidths): void {
    if (!storageKey) return;
    try {
        if (Object.keys(widths).length === 0) {
            localStorage.removeItem(ColumnWidthsPrefix + storageKey);
        } else {
            localStorage.setItem(ColumnWidthsPrefix + storageKey, JSON.stringify(widths));
        }
    } catch {
        // 存储被禁用或写满时忽略，列宽只是体验优化，不该因此让表格出错
    }
}

/** releaseCursor 解掉拖动期间加在 body 上的全局光标与禁止选中 */
function releaseCursor(): void {
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
}

/** compareSortValues 按类型选择比较方式，混合类型回退到字符串比较 */
function compareSortValues(left: string | number | bigint, right: string | number | bigint): number {
    if (typeof left === "number" && typeof right === "number") {
        return left - right;
    }
    if (typeof left === "bigint" && typeof right === "bigint") {
        return left < right ? -1 : left > right ? 1 : 0;
    }
    return collator.compare(String(left), String(right));
}

/**
 * VirtualTable 用 @tanstack/react-virtual 只渲染视口内的行
 * ABA 的对象表与 CT 的 catalog 表动辄上万行，antd Table 会把每行都挂到 DOM 上而卡住
 * 表头与行共用同一份 grid-template-columns，因此列始终对齐；表头 sticky 在同一个滚动容器内，
 * 所以列被拖宽到超出视口时表头会跟着横向滚动，不会和数据错位
 */
function VirtualTable<T>({
                             rows,
                             columns,
                             rowKey,
                             height = "100%",
                             rowHeight = DefaultRowHeight,
                             selectedKey,
                             onRowClick,
                             onRowDoubleClick,
                             emptyText,
                             defaultSort,
                             widthStorageKey,
                         }: VirtualTableProps<T>) {
    const {t} = useTranslation();
    const {token} = theme.useToken();
    const scrollRef = useRef<HTMLDivElement>(null);
    const contentRef = useRef<HTMLDivElement>(null);
    // 拖动起点要用表头单元格的实际渲染宽度，fr 与 minmax() 只有量过才知道具体是多少像素
    const headerCellsRef = useRef(new Map<string, HTMLDivElement>());
    const [sort, setSort] = useState<SortState | null>(defaultSort ?? null);
    const [widths, setWidths] = useState<ColumnWidths>(() => loadWidths(widthStorageKey));
    // 拖动中的宽度只放在 ref 里：每次指针移动都 setState 会把视口内所有单元格重渲染一遍
    // Live drag width lives in a ref: a setState per pointer move would re-render every visible cell
    const dragRef = useRef<{ key: string; startX: number; startWidth: number; width: number } | null>(null);

    const buildTemplate = useCallback(
        (map: ColumnWidths) =>
            columns.map((column) => (map[column.key] ? `${map[column.key]}px` : column.width)).join(" "),
        [columns]
    );

    const template = useMemo(() => buildTemplate(widths), [buildTemplate, widths]);
    // 拖动途中若因为别的原因重渲染，样式要停在正在拖的宽度上，不能跳回上次提交的值
    const activeDrag = dragRef.current;
    const liveTemplate = activeDrag
        ? buildTemplate({...widths, [activeDrag.key]: activeDrag.width})
        : template;

    /** commitWidth 记下一列的新宽度，只有拖动结束与键盘调整才走到这里 */
    const commitWidth = useCallback(
        (key: string, width: number) => {
            const next = {...widths, [key]: clampWidth(width)};
            setWidths(next);
            saveWidths(widthStorageKey, next);
        },
        [widths, widthStorageKey]
    );

    /** resetWidth 让一列回到列定义里的默认宽度，重新参与 fr 分摊 */
    const resetWidth = useCallback(
        (key: string) => {
            if (widths[key] === undefined) return;
            const next = {...widths};
            delete next[key];
            setWidths(next);
            saveWidths(widthStorageKey, next);
        },
        [widths, widthStorageKey]
    );

    /** measureWidth 量出这一列当前的渲染宽度，作为拖动与键盘调整的起点 */
    const measureWidth = useCallback((key: string) => {
        const cell = headerCellsRef.current.get(key);
        return cell ? cell.getBoundingClientRect().width : MinColumnWidth;
    }, []);

    const beginResize = (event: React.PointerEvent<HTMLDivElement>, key: string) => {
        if (event.button !== 0) return;
        // 手柄压在表头单元格上，这一下按压不能顺带触发排序，也不能选中表头文字
        event.preventDefault();
        event.stopPropagation();
        const startWidth = clampWidth(measureWidth(key));
        dragRef.current = {key, startX: event.clientX, startWidth, width: startWidth};
        // 指针捕获让拖出手柄甚至拖出窗口后依然收得到移动与松开事件
        event.currentTarget.setPointerCapture(event.pointerId);
        event.currentTarget.dataset.resizing = "true";
        document.body.style.cursor = "col-resize";
        document.body.style.userSelect = "none";
    };

    const moveResize = (event: React.PointerEvent<HTMLDivElement>) => {
        const drag = dragRef.current;
        if (!drag) return;
        const width = clampWidth(drag.startWidth + event.clientX - drag.startX);
        if (width === drag.width) return;
        drag.width = width;
        contentRef.current?.style.setProperty(TemplateVariable, buildTemplate({...widths, [drag.key]: width}));
    };

    const endResize = (event: React.PointerEvent<HTMLDivElement>) => {
        const drag = dragRef.current;
        if (!drag) return;
        dragRef.current = null;
        delete event.currentTarget.dataset.resizing;
        releaseCursor();
        commitWidth(drag.key, drag.width);
    };

    /** 手柄聚焦后左右方向键微调列宽，Home 恢复默认，键盘用户不必依赖拖动 */
    const resizeByKey = (event: React.KeyboardEvent<HTMLDivElement>, key: string) => {
        const step = event.shiftKey ? KeyboardLargeStep : KeyboardStep;
        if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
            event.preventDefault();
            const current = widths[key] ?? measureWidth(key);
            commitWidth(key, current + (event.key === "ArrowRight" ? step : -step));
        } else if (event.key === "Home") {
            event.preventDefault();
            resetWidth(key);
        }
    };

    // 卸载时若还按着手柄，body 上的全局光标与禁止选中必须解掉
    useEffect(() => () => {
        if (dragRef.current) releaseCursor();
    }, []);

    // 点击表头循环：升序 → 降序 → 原始顺序，第三态保留下来是因为 PathID 的物理顺序本身有排查价值
    // Header clicks cycle ascending → descending → original order, and that third state matters because the physical PathID order is worth inspecting
    const toggleSort = useCallback((key: string) => {
        setSort((current) => {
            if (!current || current.key !== key) return {key, order: "asc"};
            if (current.order === "asc") return {key, order: "desc"};
            return null;
        });
        scrollRef.current?.scrollTo({top: 0});
    }, []);

    const sortedRows = useMemo(() => {
        if (!sort) return rows;
        const column = columns.find((item) => item.key === sort.key);
        if (!column?.sortValue) return rows;
        const getValue = column.sortValue;
        const direction = sort.order === "asc" ? 1 : -1;
        // Array.prototype.sort 是稳定排序，键相同的行保持原有相对顺序
        return [...rows].sort((left, right) => direction * compareSortValues(getValue(left), getValue(right)));
    }, [rows, columns, sort]);

    const virtualizer = useVirtualizer({
        count: sortedRows.length,
        getScrollElement: () => scrollRef.current,
        estimateSize: () => rowHeight,
        overscan: 16,
        // 表头也在这个滚动容器里，先占掉一段高度，行的坐标要相应偏移
        scrollMargin: HeaderHeight,
    });

    // 过滤条件变化后行数骤减时，视口可能停在早已不存在的位置
    useEffect(() => {
        if (scrollRef.current && scrollRef.current.scrollTop > sortedRows.length * rowHeight) {
            scrollRef.current.scrollTo({top: 0});
        }
    }, [sortedRows.length, rowHeight]);

    const cellStyle = (column: VirtualColumn<T>): React.CSSProperties => ({
        padding: "0 10px",
        textAlign: column.align ?? "left",
        overflow: "hidden",
        textOverflow: "ellipsis",
        whiteSpace: "nowrap",
        display: "flex",
        alignItems: "center",
        justifyContent:
            column.align === "right" ? "flex-end" : column.align === "center" ? "center" : "flex-start",
    });

    const contentStyle: StyleWithVars = {
        [TemplateVariable]: liveTemplate,
        // 列宽之和超过视口时撑开内容区，交给外层滚动容器横向滚动，而不是把列挤变形
        minWidth: "min-content",
        minHeight: "100%",
        display: "flex",
        flexDirection: "column",
    };

    return (
        <div style={{display: "flex", flexDirection: "column", height, minHeight: 0}}>
            <div
                ref={scrollRef}
                data-virtual-scroller
                style={{flex: 1, overflow: "auto", minHeight: 0}}
            >
                <div ref={contentRef} style={contentStyle}>
                    <div
                        style={{
                            display: "grid",
                            gridTemplateColumns: `var(${TemplateVariable})`,
                            height: HeaderHeight,
                            flex: "none",
                            // 纵向滚动时贴住顶部，横向滚动时跟着列一起移动
                            position: "sticky",
                            top: 0,
                            zIndex: 2,
                            alignItems: "center",
                            // colorFillAlter 是半透明的，sticky 之后行会从表头底下透出来，
                            // 先垫一层容器底色再叠上去，视觉与原来贴在 Card 上时一致
                            backgroundColor: token.colorBgContainer,
                            backgroundImage: `linear-gradient(${token.colorFillAlter}, ${token.colorFillAlter})`,
                            borderBottom: `1px solid ${token.colorBorderSecondary}`,
                            borderTopLeftRadius: token.borderRadiusLG,
                            borderTopRightRadius: token.borderRadiusLG,
                            fontWeight: 600,
                            fontSize: token.fontSizeSM,
                            color: token.colorTextSecondary,
                        }}
                    >
                        {columns.map((column) => {
                            const sortable = Boolean(column.sortValue);
                            const active = sort?.key === column.key ? sort.order : null;
                            // 复选框与操作按钮列宽度固定，调整没有意义
                            const resizable = column.resizable !== false;
                            const label = typeof column.title === "string" ? column.title : column.key;
                            return (
                                <div
                                    key={column.key}
                                    ref={(node) => {
                                        if (node) headerCellsRef.current.set(column.key, node);
                                        else headerCellsRef.current.delete(column.key);
                                    }}
                                    style={{
                                        ...cellStyle(column),
                                        position: "relative",
                                        cursor: sortable ? "pointer" : "default",
                                        gap: 4,
                                    }}
                                    title={typeof column.title === "string" ? column.title : undefined}
                                    onClick={sortable ? () => toggleSort(column.key) : undefined}
                                >
                                    <span style={{overflow: "hidden", textOverflow: "ellipsis"}}>{column.title}</span>
                                    {sortable && (
                                        <span
                                            style={{
                                                display: "inline-flex",
                                                flexDirection: "column",
                                                lineHeight: 0,
                                                fontSize: 9,
                                                flex: "none",
                                            }}
                                        >
                                            <CaretUpFilled
                                                style={{color: active === "asc" ? token.colorPrimary : token.colorTextQuaternary}}
                                            />
                                            <CaretDownFilled
                                                style={{color: active === "desc" ? token.colorPrimary : token.colorTextQuaternary}}
                                            />
                                        </span>
                                    )}
                                    {resizable && (
                                        <div
                                            className="vt-resize-handle"
                                            role="separator"
                                            aria-orientation="vertical"
                                            aria-label={t("VirtualTable.resize_column", {name: label})}
                                            aria-valuemin={MinColumnWidth}
                                            aria-valuemax={MaxColumnWidth}
                                            aria-valuenow={widths[column.key]}
                                            tabIndex={0}
                                            title={t("VirtualTable.resize_hint")}
                                            style={{
                                                color: token.colorPrimary,
                                                "--vt-divider": token.colorBorder,
                                            } as StyleWithVars}
                                            onPointerDown={(event) => beginResize(event, column.key)}
                                            onPointerMove={moveResize}
                                            onPointerUp={endResize}
                                            onPointerCancel={endResize}
                                            onKeyDown={(event) => resizeByKey(event, column.key)}
                                            onClick={(event) => event.stopPropagation()}
                                            onDoubleClick={(event) => {
                                                event.stopPropagation();
                                                resetWidth(column.key);
                                            }}
                                        />
                                    )}
                                </div>
                            );
                        })}
                    </div>

                    {sortedRows.length === 0 ? (
                        <div style={{flex: 1, display: "flex", alignItems: "center", justifyContent: "center"}}>
                            <Empty description={emptyText} image={Empty.PRESENTED_IMAGE_SIMPLE}/>
                        </div>
                    ) : (
                        <div style={{height: virtualizer.getTotalSize(), flex: "none", position: "relative"}}>
                            {virtualizer.getVirtualItems().map((virtualRow) => {
                                const row = sortedRows[virtualRow.index];
                                const key = rowKey(row, virtualRow.index);
                                const selected = selectedKey !== undefined && selectedKey === key;
                                return (
                                    <div
                                        key={key}
                                        onClick={() => onRowClick?.(row, virtualRow.index)}
                                        onDoubleClick={() => onRowDoubleClick?.(row, virtualRow.index)}
                                        style={{
                                            position: "absolute",
                                            // start 含表头占掉的那段，减回去才是行在数据区里的位置
                                            top: virtualRow.start - HeaderHeight,
                                            left: 0,
                                            right: 0,
                                            height: virtualRow.size,
                                            display: "grid",
                                            gridTemplateColumns: `var(${TemplateVariable})`,
                                            alignItems: "center",
                                            fontSize: token.fontSizeSM,
                                            cursor: onRowClick || onRowDoubleClick ? "pointer" : "default",
                                            background: selected ? token.controlItemBgActive : undefined,
                                            borderBottom: `1px solid ${token.colorBorderSecondary}`,
                                        }}
                                    >
                                        {columns.map((column) => {
                                            // render 的结果复用一次：字符串内容顺带作为 title，让被截断的单元格可悬停查看
                                            const content = column.render(row, virtualRow.index);
                                            return (
                                                <div
                                                    key={column.key}
                                                    className={column.mono ? "mono" : undefined}
                                                    style={cellStyle(column)}
                                                    title={typeof content === "string" ? content : undefined}
                                                >
                                                    {content}
                                                </div>
                                            );
                                        })}
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

export default VirtualTable;
