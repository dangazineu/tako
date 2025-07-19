package com.tako.app_c;

import com.tako.lib_b.Consumer;
import org.junit.Test;
import static org.junit.Assert.*;

public class AppTest {
    @Test
    public void testApp() {
        assertEquals("hello", new Consumer().consume());
    }
}
