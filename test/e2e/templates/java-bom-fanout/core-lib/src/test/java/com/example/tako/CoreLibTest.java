package com.example.tako;

import org.junit.Test;
import static org.junit.Assert.*;

/**
 * Test class for CoreLib.
 */
public class CoreLibTest {
    
    @Test
    public void testGetVersion() {
        assertEquals("1.0.0", CoreLib.getVersion());
    }
    
    @Test
    public void testGetCoreMessage() {
        CoreLib coreLib = new CoreLib();
        String message = coreLib.getCoreMessage();
        assertTrue(message.contains("Hello from CoreLib"));
        assertTrue(message.contains("v1.0.0"));
    }
    
    @Test
    public void testCalculateValue() {
        CoreLib coreLib = new CoreLib();
        assertEquals(10, coreLib.calculateValue(5));
        assertEquals(0, coreLib.calculateValue(0));
        assertEquals(-4, coreLib.calculateValue(-2));
    }
}