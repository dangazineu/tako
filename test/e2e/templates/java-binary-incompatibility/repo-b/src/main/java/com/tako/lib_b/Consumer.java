package com.tako.lib_b;

import com.tako.lib_a.SubClass;

public class Consumer {
    public String consume() {
        return new SubClass().myMethod();
    }
}
