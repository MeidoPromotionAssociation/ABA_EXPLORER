// 主题
export const ThemeModeKey = "ThemeMode"; // 存储主题模式（system / light / dark）

export const ThemeColorKey = "ThemeColor"; // 存储自定义主题色（hex，空表示默认色）

// 语言
export const LanguageKey = "i18nextLng"; // i18next-browser-languagedetector 使用的语言键

// 版本检查
export const LastUpdateCheckTimeKey = "LastUpdateCheckTime"; // 上次检查更新的时间

export const NewVersionAvailableKey = "NewVersionAvailable"; // 是否有新版本可用

export const LatestVersionKey = "LatestVersion"; // 最新版本号

export const UpdateRetryKey = "UpdateRetry"; // 上次检查失败后是否进入重试间隔

// 工作区
export const RecentFilesKey = "RecentFiles"; // 最近打开的文件路径列表

export const AbaTabKey = "AbaExplorerTab"; // ABA 页当前标签

export const CtTabKey = "CtExplorerTab"; // CT 页当前标签

export const ContainerOverviewOpenKey = "ContainerOverviewOpen"; // 容器页概览是否展开（默认折叠）

export const CtOverviewOpenKey = "CtOverviewOpen"; // 内容表页概览是否展开（默认折叠）

export const AutoGenerateCtKey = "AutoGenerateCt"; // 打包后是否自动生成配套 .ct

// 全局搜索
export const SearchRootKey = "SearchIndexRoot"; // 上次索引的根目录

export const SearchDeepKey = "SearchIndexDeep"; // 是否解析容器内部名（.menu/.mate/.pmat）

// 表格
export const ColumnWidthsPrefix = "ColumnWidths:"; // 用户拖动调整过的列宽，后接表标识

// 各虚拟表的列宽标识，拼在 ColumnWidthsPrefix 后面
export const AssetTableWidthsKey = "assets"; // 容器页对象表

export const BlockTableWidthsKey = "blocks"; // 容器页数据块表

export const DirectoryTableWidthsKey = "directories"; // 容器页目录条目表

export const CatalogItemTableWidthsKey = "catalogItems"; // 内容表页 catalog 条目表

export const ExtensionListWidthsKey = "extensionNames"; // 内容表页扩展名分组表

export const UnpackedFileTableWidthsKey = "unpackedFiles"; // 解包产物页文件表

export const SearchHitTableWidthsKey = "searchHits"; // 全局搜索页结果表

// 最近打开列表的上限
export const RecentFilesLimit = 12;
