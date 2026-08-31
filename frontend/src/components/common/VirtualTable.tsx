import React, {useCallback, useEffect, useMemo, useRef, useState} from "react";
import {Empty, theme} from "antd";
import {CaretDownFilled, CaretUpFilled} from "@ant-design/icons";
import {useVirtualizer} from "@tanstack/react-virtual";

/** 排序方向，null 表示回到数据的原始顺序 */
export type SortOrder = "asc" | "desc";

export interface SortState {
    key: string;
    order: SortOrder;
}

/**
 * VirtualColumn 一列的定义
 * width 直接作为 CSS grid 的轨道尺寸，可用 px、fr 或 minmax()
 */
export interface VirtualColumn<T> {
    key: string;
    title: React.ReactNode;
    width: string;
    align?: "left" | "right" | "center";
    /** 用等宽字体渲染，适合 PathID、哈希与偏移 */
    mono?: boolean;
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
}

const DefaultRowHeight = 34;

// 复用一个 Collator：numeric 让 asset_2 排在 asset_10 之前，逐次 new 在上万行上会明显变慢
// One shared Collator: numeric puts asset_2 before asset_10, and constructing it per comparison is measurably slow at scale
const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: "base"});

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
 * 表头与行共用同一份 grid-template-columns，因此列始终对齐
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
                         }: VirtualTableProps<T>) {
    const {token} = theme.useToken();
    const scrollRef = useRef<HTMLDivElement>(null);
    const [sort, setSort] = useState<SortState | null>(defaultSort ?? null);

    const template = useMemo(() => columns.map((column) => column.width).join(" "), [columns]);

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

    return (
        <div style={{display: "flex", flexDirection: "column", height, minHeight: 0}}>
            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: template,
                    height: 36,
                    flex: "none",
                    alignItems: "center",
                    background: token.colorFillAlter,
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
                    return (
                        <div
                            key={column.key}
                            style={{...cellStyle(column), cursor: sortable ? "pointer" : "default", gap: 4}}
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
                        </div>
                    );
                })}
            </div>

            {sortedRows.length === 0 ? (
                <div style={{flex: 1, display: "flex", alignItems: "center", justifyContent: "center"}}>
                    <Empty description={emptyText} image={Empty.PRESENTED_IMAGE_SIMPLE}/>
                </div>
            ) : (
                <div
                    ref={scrollRef}
                    data-virtual-scroller
                    style={{flex: 1, overflow: "auto", minHeight: 0}}
                >
                    <div style={{height: virtualizer.getTotalSize(), position: "relative"}}>
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
                                        top: virtualRow.start,
                                        left: 0,
                                        right: 0,
                                        height: virtualRow.size,
                                        display: "grid",
                                        gridTemplateColumns: template,
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
                </div>
            )}
        </div>
    );
}

export default VirtualTable;
