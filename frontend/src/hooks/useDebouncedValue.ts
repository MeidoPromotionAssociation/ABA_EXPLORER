import {useEffect, useState} from "react";

/**
 * useDebouncedValue 延迟跟随一个值的变化
 * 搜索框直接把每次按键都拿去过滤上万行的表格会卡手，
 * 输入框仍绑定即时值保证打字流畅，过滤只用这里返回的稳定值
 */
export function useDebouncedValue<T>(value: T, delay = 200): T {
    const [debounced, setDebounced] = useState(value);

    useEffect(() => {
        const timer = setTimeout(() => setDebounced(value), delay);
        return () => clearTimeout(timer);
    }, [value, delay]);

    return debounced;
}

export default useDebouncedValue;
