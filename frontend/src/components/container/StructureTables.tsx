import React from "react";
import {Tag} from "antd";
import {useTranslation} from "react-i18next";
import VirtualTable, {VirtualColumn} from "../common/VirtualTable";
import type {
    AbaBlock,
    AbaDirectory,
} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import {formatBytes, formatHex, formatNumber, percent} from "../../utils/format";

/** compressionColor 给压缩方式一个稳定的配色，未压缩用默认灰 */
function compressionColor(compression: string): string | undefined {
    switch (compression) {
        case "LZ4":
            return "blue";
        case "LZ4HC":
            return "geekblue";
        case "LZMA":
            return "purple";
        default:
            return undefined;
    }
}

/**
 * BlockTable 压缩数据块表
 * 一个大 ABA 的块数可达数千，同样走虚拟滚动
 */
export const BlockTable: React.FC<{ blocks: AbaBlock[] }> = ({blocks}) => {
    const {t} = useTranslation();

    const columns: VirtualColumn<AbaBlock>[] = [
        {
            key: "index",
            title: "#",
            width: "70px",
            align: "right",
            mono: true,
            sortValue: (row) => row.index,
            render: (row) => formatNumber(row.index),
        },
        {
            key: "compression",
            title: t("StructureTables.compression"),
            width: "minmax(110px, 1fr)",
            sortValue: (row) => row.compression,
            render: (row) => <Tag color={compressionColor(row.compression)} style={{marginInlineEnd: 0}}>{row.compression}</Tag>,
        },
        {
            key: "compressedSize",
            title: t("StructureTables.compressed"),
            width: "minmax(110px, 1fr)",
            align: "right",
            mono: true,
            sortValue: (row) => row.compressedSize,
            render: (row) => formatBytes(row.compressedSize),
        },
        {
            key: "decompressedSize",
            title: t("StructureTables.decompressed"),
            width: "minmax(110px, 1fr)",
            align: "right",
            mono: true,
            sortValue: (row) => row.decompressedSize,
            render: (row) => formatBytes(row.decompressedSize),
        },
        {
            key: "ratio",
            title: t("StructureTables.ratio"),
            width: "90px",
            align: "right",
            mono: true,
            sortValue: (row) => (row.decompressedSize ? row.compressedSize / row.decompressedSize : 0),
            render: (row) => percent(row.compressedSize, row.decompressedSize),
        },
        {
            key: "streamed",
            title: t("StructureTables.streamed"),
            width: "100px",
            align: "center",
            sortValue: (row) => (row.streamed ? 1 : 0),
            render: (row) => (row.streamed ? <Tag color="orange" style={{marginInlineEnd: 0}}>{t("Common.yes")}</Tag> : ""),
        },
        {
            key: "flags",
            title: t("StructureTables.flags"),
            width: "100px",
            align: "right",
            mono: true,
            sortValue: (row) => row.flags,
            render: (row) => formatHex(row.flags),
        },
    ];

    return (
        <VirtualTable
            rows={blocks}
            columns={columns}
            rowKey={(row) => String(row.index)}
            defaultSort={{key: "index", order: "asc"}}
            emptyText={t("StructureTables.no_blocks")}
        />
    );
};

/**
 * DirectoryTable 目录条目表
 * Flags 的 0x04 位表示条目是序列化 AssetsFile，其余条目是 .ress 之类的流式资源
 */
export const DirectoryTable: React.FC<{ directories: AbaDirectory[] }> = ({directories}) => {
    const {t} = useTranslation();

    const columns: VirtualColumn<AbaDirectory>[] = [
        {
            key: "index",
            title: "#",
            width: "70px",
            align: "right",
            mono: true,
            sortValue: (row) => row.index,
            render: (row) => formatNumber(row.index),
        },
        {
            key: "name",
            title: t("StructureTables.name"),
            width: "minmax(200px, 3fr)",
            sortValue: (row) => row.name,
            render: (row) => row.name,
        },
        {
            key: "offset",
            title: t("StructureTables.offset"),
            width: "minmax(110px, 1fr)",
            align: "right",
            mono: true,
            sortValue: (row) => row.offset,
            render: (row) => formatNumber(row.offset),
        },
        {
            key: "size",
            title: t("StructureTables.size"),
            width: "minmax(110px, 1fr)",
            align: "right",
            mono: true,
            sortValue: (row) => row.decompressedSize,
            render: (row) => formatBytes(row.decompressedSize),
        },
        {
            key: "serialized",
            title: t("StructureTables.serialized"),
            width: "130px",
            align: "center",
            sortValue: (row) => (row.serialized ? 1 : 0),
            render: (row) =>
                row.serialized ? (
                    <Tag color="green" style={{marginInlineEnd: 0}}>{t("StructureTables.assets_file")}</Tag>
                ) : (
                    <Tag style={{marginInlineEnd: 0}}>{t("StructureTables.stream")}</Tag>
                ),
        },
        {
            key: "flags",
            title: t("StructureTables.flags"),
            width: "100px",
            align: "right",
            mono: true,
            sortValue: (row) => row.flags,
            render: (row) => formatHex(row.flags),
        },
    ];

    return (
        <VirtualTable
            rows={directories}
            columns={columns}
            rowKey={(row) => String(row.index)}
            defaultSort={{key: "name", order: "asc"}}
            emptyText={t("StructureTables.no_directories")}
        />
    );
};
