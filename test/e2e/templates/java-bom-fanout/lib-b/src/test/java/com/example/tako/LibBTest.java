package com.example.tako;

import org.junit.Test;
import static org.junit.Assert.*;

/**
 * Test class for LibB.
 */
public class LibBTest {
    
    private final LibB libB = new LibB();
    
    @Test
    public void testGetVersion() {
        assertEquals("1.0.0", LibB.getVersion());
    }
    
    @Test
    public void testGetLibBMessage() {
        String message = libB.getLibBMessage();
        assertTrue(message.contains("LibB v1.0.0"));
        assertTrue(message.contains("Hello from CoreLib"));
    }
    
    @Test
    public void testAdvancedCalculation() {
        // LibB should multiply CoreLib's result (input * 2) by 3
        assertEquals(30, libB.advancedCalculation(5)); // (5 * 2) * 3 = 30
        assertEquals(0, libB.advancedCalculation(0)); // (0 * 2) * 3 = 0
        assertEquals(-12, libB.advancedCalculation(-2)); // (-2 * 2) * 3 = -12
    }
    
    @Test
    public void testGetCoreLibVersion() {
        assertEquals("1.0.0", libB.getCoreLibVersion());
    }
    
    @Test
    public void testIsPositive() {
        assertTrue(libB.isPositive(1));
        assertTrue(libB.isPositive(100));
        assertFalse(libB.isPositive(0));
        assertFalse(libB.isPositive(-1));
    }
}