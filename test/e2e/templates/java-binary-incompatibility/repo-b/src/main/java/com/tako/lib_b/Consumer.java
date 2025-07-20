package com.tako.lib_b;

import com.tako.lib_a.Producer;

public class Consumer {
    public String consume() {
        return new Producer().myMethod();
    }
}
